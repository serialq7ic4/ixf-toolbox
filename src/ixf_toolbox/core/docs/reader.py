from __future__ import annotations

import base64
import contextlib
from collections import Counter
from datetime import datetime, timezone
import gzip
import json
import os
from pathlib import Path
import re
from typing import Any
from urllib.parse import parse_qs, urlencode, urlparse
from zoneinfo import ZoneInfo

import requests

from ixf_toolbox.core.cookies import DEFAULT_COOKIES, load_cookie_objects
from ixf_toolbox.core.docs.assets import ImageAssetWriter
from ixf_toolbox.core.docs.converters.docx_markdown import ConversionOptions
from ixf_toolbox.core.docs.converters.docx_markdown import convert_docx_client_vars
from ixf_toolbox.core.docs.converters.docx_markdown import extract_text


DEFAULT_SPACE_API = "https://internal-api-space.xfchat.iflytek.com"
USER_AGENT = (
    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
    "AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0 Safari/537.36"
)


def build_session(cookies: list[dict[str, Any]]) -> requests.Session:
    session = requests.Session()
    for cookie in cookies:
        session.cookies.set(
            cookie["name"],
            cookie["value"],
            domain=cookie.get("domain"),
            path=cookie.get("path", "/"),
        )
    return session


def csrf_from(cookies: list[dict[str, Any]]) -> str:
    for cookie in cookies:
        if cookie.get("name") == "_csrf_token":
            return str(cookie.get("value"))
    raise ValueError("Cookie jar does not contain _csrf_token.")


def is_remote(source: str) -> bool:
    return source.startswith("http://") or source.startswith("https://")


def origin_for(url: str) -> str:
    parsed = urlparse(url)
    return f"{parsed.scheme}://{parsed.netloc}"


def common_headers(origin: str, csrf_token: str, referer: str) -> dict[str, str]:
    return {
        "User-Agent": USER_AGENT,
        "Origin": origin,
        "Referer": referer,
        "X-CSRFToken": csrf_token,
    }


def common_lgw_headers(origin: str, lgw_csrf_token: str, referer: str) -> dict[str, str]:
    return {
        "User-Agent": USER_AGENT,
        "Origin": origin,
        "Referer": referer,
        "accept": "application/json,text/plain,*/*",
        "x-lgw-csrf-token": lgw_csrf_token,
        "x-requested-with": "XMLHttpRequest",
        "okr-language": "zh-CN",
        "okr-timezone": "Asia/Shanghai",
    }


def fetch_html(session: requests.Session, url: str, csrf_token: str) -> str:
    origin = origin_for(url)
    response = session.get(url, headers=common_headers(origin, csrf_token, url), timeout=30)
    response.raise_for_status()
    return response.text


def merge_client_vars_page(target: dict[str, Any], page: dict[str, Any]) -> None:
    for key, value in page.items():
        if key == "block_map" and isinstance(value, dict):
            block_map = target.setdefault("block_map", {})
            if isinstance(block_map, dict):
                block_map.update(value)
                continue
        if key not in {"has_more", "cursor"}:
            target[key] = value


def client_vars(
    session: requests.Session,
    space_api: str,
    page_id: str,
    origin: str,
    csrf_token: str,
) -> dict[str, Any]:
    base_url = f"{space_api.rstrip('/')}/space/api/docx/pages/client_vars"
    referer = f"{origin}/docx/{page_id}"
    data: dict[str, Any] = {}
    cursor = ""
    while True:
        query = f"id={page_id}&open_type=1"
        if cursor:
            query = f"{query}&mode=4&cursor={cursor}"
        response = session.get(
            f"{base_url}?{query}",
            headers=common_headers(origin, csrf_token, referer),
            timeout=30,
        )
        response.raise_for_status()
        payload = response.json()
        if payload.get("code") != 0:
            raise RuntimeError(f"client_vars failed: {payload}")
        page = payload["data"]
        merge_client_vars_page(data, page)
        cursor = str(page.get("cursor") or "")
        if not page.get("has_more") or not cursor:
            return data


def bitable_client_vars(
    session: requests.Session,
    origin: str,
    base_token: str,
    csrf_token: str,
    referer: str,
) -> dict[str, Any]:
    response = session.get(
        (
            f"{origin}/space/api/v1/bitable/{base_token}/clientvars"
            "?tableID=&viewID=&recordLimit=2000&ondemandLimit=200"
            "&needBase=true&viewLazyLoad=true&ondemandVer=2"
            "&openType=0&noMissCS=true&optimizationFlag=1"
        ),
        headers=common_headers(origin, csrf_token, referer),
        timeout=60,
    )
    response.raise_for_status()
    payload = response.json()
    if payload.get("code") != 0:
        raise RuntimeError(f"bitable clientvars failed: {payload}")
    return payload["data"]


def extract_doc_token(url: str, html: str) -> str:
    parsed = urlparse(url)
    docx_match = re.search(r"/docx/([^/?#]+)", parsed.path)
    if docx_match:
        return docx_match.group(1)

    patterns = [
        r'obj_token":"([^"]+)"',
        r'token":"(dox[a-zA-Z0-9]+)"',
        r'url_token":"(dox[a-zA-Z0-9]+)"',
    ]
    for pattern in patterns:
        match = re.search(pattern, html)
        if match:
            return match.group(1)
    raise RuntimeError("Unable to locate a doc token in page HTML.")


def extract_balanced_object(text: str, anchor: str) -> str:
    start = text.find(anchor)
    if start == -1:
        raise RuntimeError(f"Anchor not found: {anchor}")
    start += len(anchor)
    while start < len(text) and text[start].isspace():
        start += 1
    if start >= len(text) or text[start] != "{":
        raise RuntimeError(f"Expected '{{' after anchor: {anchor}")

    depth = 0
    in_string = False
    escape = False
    for idx in range(start, len(text)):
        ch = text[idx]
        if in_string:
            if escape:
                escape = False
            elif ch == "\\":
                escape = True
            elif ch == '"':
                in_string = False
            continue
        if ch == '"':
            in_string = True
            continue
        if ch == "{":
            depth += 1
        elif ch == "}":
            depth -= 1
            if depth == 0:
                return text[start : idx + 1]
    raise RuntimeError(f"Unterminated object after anchor: {anchor}")


def extract_current_space_wiki(html: str) -> dict[str, Any]:
    return json.loads(extract_balanced_object(html, "current_space_wiki = Object("))


def decode_gzip_json(encoded: str) -> Any:
    raw = gzip.decompress(base64.b64decode(encoded))
    return json.loads(raw)


def split_sheet_block_token(sheet_block_token: str) -> tuple[str, str]:
    if "_" not in sheet_block_token:
        raise ValueError(f"Invalid embedded sheet token: {sheet_block_token}")
    workbook_token, sheet_id = sheet_block_token.rsplit("_", 1)
    if not workbook_token or not sheet_id:
        raise ValueError(f"Invalid embedded sheet token: {sheet_block_token}")
    return workbook_token, sheet_id


def fetch_embedded_sheet(
    session: requests.Session,
    origin: str,
    host_page_token: str,
    sheet_block_token: str,
    csrf_token: str,
) -> dict[str, Any]:
    workbook_token, sheet_id = split_sheet_block_token(sheet_block_token)
    response = session.post(
        (
            f"{origin}/space/api/v3/sheet/client_vars"
            f"?synced_block_host_token={host_page_token}&synced_block_host_type=22"
        ),
        json={
            "memberId": 0,
            "schemaVersion": 9,
            "openType": 1,
            "token": workbook_token,
            "sheetRange": {"sheetId": sheet_id},
            "clientVersion": "v0.0.1",
        },
        headers=common_headers(origin, csrf_token, f"{origin}/docx/{host_page_token}"),
        timeout=60,
    )
    response.raise_for_status()
    payload = response.json()
    if payload.get("code") != 0:
        raise RuntimeError(f"sheet client_vars failed: {payload}")
    return payload["data"]


def normalize_sheet_text(text: str) -> str:
    text = text.replace("\r\n", "\n").replace("\r", "\n")
    text = re.sub(r"\n{3,}", "\n\n", text)
    return text.strip()


def should_start_rich_text_line(piece: str, next_piece: str) -> bool:
    compact = piece.strip()
    if not compact or len(compact) > 24:
        return False
    if compact.endswith((":", "：")):
        return True
    return next_piece.lstrip().startswith((":", "："))


def render_sheet_value(value: Any) -> str:
    if value is None:
        return ""
    if isinstance(value, str):
        return value
    if isinstance(value, (int, float, bool)):
        return str(value)
    if isinstance(value, list):
        rendered_parts = [render_sheet_value(item) for item in value]
        text_parts: list[str] = []
        for index, rendered in enumerate(rendered_parts):
            if rendered:
                next_piece = rendered_parts[index + 1] if index + 1 < len(rendered_parts) else ""
                if (
                    text_parts
                    and not text_parts[-1].endswith("\n")
                    and should_start_rich_text_line(rendered, next_piece)
                ):
                    text_parts.append("\n")
                text_parts.append(rendered)
        return "".join(text_parts)
    if isinstance(value, dict):
        if "text" in value:
            return render_sheet_value(value.get("text"))
        for key in ("value", "formattedValue", "displayValue"):
            if key in value:
                rendered = render_sheet_value(value.get(key))
                if rendered:
                    return rendered
        if set(value.keys()) <= {"styleId", "type"}:
            return ""
        return json.dumps(value, ensure_ascii=False)
    return str(value)


def cell_to_text(cell: Any) -> str:
    if cell is None:
        return ""
    if not isinstance(cell, dict):
        return normalize_sheet_text(render_sheet_value(cell))
    for key in ("value", "formattedValue", "displayValue"):
        if key in cell:
            return normalize_sheet_text(render_sheet_value(cell.get(key)))
    if set(cell.keys()) <= {"styleId", "type"}:
        return ""
    return ""


def escape_tsv_cell(value: str) -> str:
    return value.replace("\\", "\\\\").replace("\t", "\\t").replace("\r\n", "\n").replace("\n", "\\n")


def bitable_field_option_map(field: dict[str, Any]) -> dict[str, str]:
    options = field.get("property", {}).get("options", [])
    return {
        str(option.get("id")): str(option.get("name", ""))
        for option in options
        if isinstance(option, dict) and option.get("id")
    }


def bitable_user_map(client_vars_data: dict[str, Any]) -> dict[str, str]:
    user_map: dict[str, str] = {}
    users = client_vars_data.get("users", {})
    if isinstance(users, dict):
        for user_id, user in users.items():
            if not isinstance(user, dict):
                continue
            name = user.get("name") or user.get("enName") or user.get("displayName")
            if name:
                user_map[str(user_id)] = str(name)
    old_users = client_vars_data.get("oldSchema", {}).get("users", {})
    if isinstance(old_users, dict):
        for user_id, user in old_users.items():
            if not isinstance(user, dict):
                continue
            name = user.get("name") or user.get("enName") or user.get("displayName")
            if name and str(user_id) not in user_map:
                user_map[str(user_id)] = str(name)
    return user_map


def format_bitable_datetime(value: Any, field: dict[str, Any], tz_name: str) -> str:
    if not isinstance(value, (int, float)):
        return render_sheet_value(value)
    timestamp = float(value)
    if timestamp > 10**12:
        timestamp /= 1000
    try:
        tz = ZoneInfo(tz_name) if tz_name else timezone.utc
    except Exception:
        tz = timezone.utc
    dt = datetime.fromtimestamp(timestamp, tz)
    prop = field.get("property", {})
    date_format = str(prop.get("dateFormat", "yyyy/MM/dd"))
    time_format = str(prop.get("timeFormat", "")).strip()
    fmt = date_format.replace("yyyy", "%Y").replace("MM", "%m").replace("dd", "%d")
    if time_format:
        fmt = f"{fmt} {time_format.replace('HH', '%H').replace('mm', '%M').replace('ss', '%S')}"
    return dt.strftime(fmt)


def render_bitable_value(
    value: Any,
    field: dict[str, Any],
    user_map: dict[str, str],
    tz_name: str,
) -> str:
    field_type = int(field.get("type", 0) or 0)
    if value is None:
        return ""
    if field_type == 3:
        option_map = bitable_field_option_map(field)
        if isinstance(value, list):
            return ", ".join(option_map.get(str(item), str(item)) for item in value if item is not None)
        return option_map.get(str(value), str(value))
    if field_type == 5:
        return format_bitable_datetime(value, field, tz_name)
    if field_type == 11:
        if isinstance(value, list):
            return ", ".join(user_map.get(str(item), str(item)) for item in value if item is not None)
        return user_map.get(str(value), str(value))
    return render_sheet_value(value)


def render_bitable_as_tsv(
    client_vars_data: dict[str, Any],
    base_token: str,
) -> tuple[list[str], Counter[str]]:
    old_schema = decode_gzip_json(client_vars_data["oldSchema"]["gzipSchema"])
    base = old_schema["base"]
    table_data = old_schema["data"]["table"]
    record_map = old_schema["data"]["recordMap"]
    table_id = str(base["tables"][0])
    table_name = str(base.get("tableInfos", {}).get(table_id, {}).get("name", "") or table_id)
    views = table_data.get("views", [])
    view_map = table_data.get("viewMap", {})
    selected_view: dict[str, Any] | None = None
    for view_id in views:
        view = view_map.get(view_id)
        if isinstance(view, dict) and int(view.get("type", 0) or 0) == 1:
            selected_view = view
            break
    if selected_view is None and views:
        selected_view = view_map.get(views[0])
    if not isinstance(selected_view, dict):
        raise RuntimeError("Unable to locate a renderable bitable view.")

    view_id = str(selected_view.get("id", ""))
    view_name = str(selected_view.get("name", "") or view_id)
    field_ids = [str(field_id) for field_id in selected_view.get("property", {}).get("fields", [])]
    record_ids = [str(record_id) for record_id in selected_view.get("property", {}).get("records", [])]
    field_map = table_data.get("fieldMap", {})
    user_map = bitable_user_map(client_vars_data)
    tz_name = str(base.get("timezone", "") or client_vars_data.get("timeZone", "") or "")

    headers = [str(field_map.get(field_id, {}).get("name", field_id)) for field_id in field_ids]
    lines = [
        (
            f"[bitable-meta base_token={base_token} table_id={table_id} "
            f'table_name="{table_name}" view_id={view_id} '
            f'view_name="{view_name}" rows={len(record_ids)} cols={len(field_ids)} views={len(views)}]'
        ),
        "```tsv",
        "\t".join(escape_tsv_cell(header) for header in headers),
    ]
    for record_id in record_ids:
        record = record_map.get(record_id, {})
        row: list[str] = []
        for field_id in field_ids:
            field = field_map.get(field_id, {})
            cell = record.get(field_id, {})
            value = cell.get("value") if isinstance(cell, dict) else cell
            rendered = render_bitable_value(value, field, user_map, tz_name)
            row.append(escape_tsv_cell(normalize_sheet_text(rendered)))
        lines.append("\t".join(row))
    lines.extend(["```", ""])
    counts = Counter(
        {
            "bitable": 1,
            "bitable_views": len(views),
            "bitable_fields": len(field_ids),
            "bitable_records": len(record_ids),
        }
    )
    return lines, counts


def render_embedded_sheet_as_tsv(
    session: requests.Session,
    origin: str,
    host_page_token: str,
    sheet_block_token: str,
    csrf_token: str,
) -> list[str]:
    data = fetch_embedded_sheet(session, origin, host_page_token, sheet_block_token, csrf_token)
    workbook_token, sheet_id = split_sheet_block_token(sheet_block_token)
    formerly_schema = data["formerlySchema"]
    clientvars = formerly_schema["clientvars"]
    snapshot = decode_gzip_json(clientvars["gzip_snapshot"])
    sheet_meta = snapshot.get("sheets", {}).get(sheet_id, {})
    row_count = int(sheet_meta.get("rowCount", 0) or 0)
    column_count = int(sheet_meta.get("columnCount", 0) or 0)

    row_map: dict[int, list[str]] = {}
    for block in clientvars.get("extra_data", {}).get("blocks", []):
        datatable = decode_gzip_json(block["gzip_datatable"])
        start_row = int(block.get("row", 0) or 0)
        rows = datatable.get("rows", [])
        for offset, row in enumerate(rows):
            columns = row.get("columns", [])
            row_map[start_row + offset] = [cell_to_text(column) for column in columns]
            column_count = max(column_count, len(row_map[start_row + offset]))

    max_row = max(row_map.keys(), default=-1)
    if row_count <= 0:
        row_count = max_row + 1

    lines = [
        (
            f"[sheet-meta workbook_token={workbook_token} "
            f"sheet_id={sheet_id} rows={row_count} cols={column_count}]"
        ),
        "```tsv",
    ]
    for row_index in range(row_count):
        values = row_map.get(row_index, [])
        if len(values) < column_count:
            values = values + [""] * (column_count - len(values))
        lines.append("\t".join(escape_tsv_cell(value) for value in values))
    lines.extend(["```", ""])
    return lines


def normalize_lines(lines: list[str]) -> list[str]:
    cleaned = [line.rstrip() for line in lines]
    out: list[str] = []
    blank_run = 0
    for line in cleaned:
        if not line.strip():
            blank_run += 1
            if blank_run <= 1:
                out.append("")
            continue
        blank_run = 0
        out.append(line)
    while out and not out[-1].strip():
        out.pop()
    return out


def parse_jsonish(value: Any) -> Any:
    if isinstance(value, str):
        try:
            return json.loads(value)
        except json.JSONDecodeError:
            return value
    return value


def text_from_rich_value(value: Any) -> str:
    value = parse_jsonish(value)
    if value is None:
        return ""
    if isinstance(value, str):
        return value.replace("\u200b", "").strip()
    if isinstance(value, dict):
        if "blocks" in value and isinstance(value["blocks"], list):
            return "\n".join(
                str(block.get("text", ""))
                for block in value["blocks"]
                if isinstance(block, dict)
            ).replace("\u200b", "").strip()
        if "0" in value and isinstance(value["0"], dict):
            ops = value["0"].get("ops", [])
            if isinstance(ops, list):
                return "".join(
                    str(op.get("insert", ""))
                    for op in ops
                    if isinstance(op, dict)
                ).replace("\u200b", "").strip()
        if "text" in value:
            return text_from_rich_value(value.get("text"))
    return ""


def okr_item_text(item: dict[str, Any]) -> str:
    for key in ("content_v2", "contentV2", "content", "name"):
        text = text_from_rich_value(item.get(key))
        if text:
            return text
    return ""


def okr_owner_name(detail: dict[str, Any]) -> str:
    owner_info = detail.get("owner_info") or detail.get("ownerInfo") or {}
    if not isinstance(owner_info, dict):
        return ""
    user_info = owner_info.get("user_info") or owner_info.get("userInfo") or {}
    if not isinstance(user_info, dict):
        return ""
    locale_names = user_info.get("locale_names") or user_info.get("localeNames") or {}
    if isinstance(locale_names, dict):
        for key in ("zh", "en", "ja"):
            name = str(locale_names.get(key) or "").strip()
            if name:
                return name
    for key in ("name", "displayName", "display_name"):
        name = str(user_info.get(key) or "").strip()
        if name:
            return name
    return ""


def okr_id_from_url(source: str) -> str:
    parsed = urlparse(source)
    query = parse_qs(parsed.query)
    for key in ("okrId", "okr_id"):
        value = query.get(key, [""])[0]
        if value:
            return value
    raise RuntimeError("Unable to locate okrId in OKR URL.")


def ensure_lgw_csrf_token(session: requests.Session) -> str:
    response = session.get("https://www.xfchat.iflytek.com/lgw/csrf_token", timeout=30)
    response.raise_for_status()
    token = str(session.cookies.get("lgw_csrf_token", "") or "")
    if not token:
        raise RuntimeError("Unable to obtain lgw_csrf_token from local session cookies.")
    return token


def okr_progress_text(progress: Any) -> str:
    if not isinstance(progress, dict):
        return ""
    percent = progress.get("percent")
    if percent is None:
        return ""
    try:
        numeric = float(percent)
    except (TypeError, ValueError):
        return ""
    if numeric.is_integer():
        return f"{int(numeric)}%"
    return f"{numeric:g}%"


def okr_response_error(operation: str, payload: object) -> str:
    if not isinstance(payload, dict):
        return f"{operation} returned an unexpected payload type: {type(payload).__name__}."
    code = payload.get("code")
    if code not in {0, None}:
        return f"{operation} failed with code {code}."
    keys = ", ".join(sorted(str(key) for key in payload))
    return f"{operation} returned an unexpected payload shape; keys: {keys or '(none)'}."


def render_okr_markdown(
    detail: dict[str, Any],
    okr_id: str,
) -> tuple[str, str, str, Counter[str]]:
    period = str(detail.get("name") or detail.get("period_name") or detail.get("periodName") or "").strip()
    owner = okr_owner_name(detail)
    title_parts = ["OKR"]
    if owner:
        title_parts.append(owner)
    if period:
        title_parts.append(period)
    title = " - ".join(title_parts)

    objective_list = detail.get("objective_list") or detail.get("objectiveList") or []
    if not isinstance(objective_list, list):
        objective_list = []

    lines = [f"# {title}", "", f"[okr id={okr_id} objectives={len(objective_list)}]", ""]
    key_result_count = 0
    for objective_index, objective in enumerate(objective_list, start=1):
        if not isinstance(objective, dict):
            continue
        objective_text = okr_item_text(objective) or str(objective.get("id") or "").strip()
        lines.extend([f"## O{objective_index} {objective_text}", ""])
        kr_list = objective.get("kr_list") or objective.get("krList") or []
        if not isinstance(kr_list, list):
            kr_list = []
        for kr_index, kr in enumerate(kr_list, start=1):
            if not isinstance(kr, dict):
                continue
            key_result_count += 1
            kr_text = okr_item_text(kr) or str(kr.get("id") or "").strip()
            progress = okr_progress_text(kr.get("progress_rate") or kr.get("progressRate"))
            suffix = f" _(progress: {progress})_" if progress else ""
            lines.append(f"- KR{kr_index}: {kr_text}{suffix}")
        lines.append("")

    counts = Counter({"objectives": len(objective_list), "key_results": key_result_count})
    return title, okr_id, "\n".join(normalize_lines(lines)).strip() + "\n", counts


def read_okr(
    session: requests.Session,
    source: str,
) -> tuple[str, str, str, Counter[str]]:
    okr_id = okr_id_from_url(source)
    origin = origin_for(source)
    lgw_csrf_token = ensure_lgw_csrf_token(session)
    query = urlencode({"okr_id": okr_id, "withoutAddVisitLog": "true"})
    response = session.get(
        f"{origin}/okrx/api/okr/owner/aggr_detail/?{query}",
        headers=common_lgw_headers(origin, lgw_csrf_token, source),
        timeout=30,
    )
    response.raise_for_status()
    payload = response.json()
    if not isinstance(payload, dict) or payload.get("code") not in {0, None}:
        raise RuntimeError(okr_response_error("OKR aggr_detail", payload))
    detail = (
        payload.get("okr_detail_data")
        or payload.get("okrDetailData")
        or payload.get("data")
        or {}
    )
    if not isinstance(detail, dict):
        raise RuntimeError(okr_response_error("OKR aggr_detail", payload))
    return render_okr_markdown(detail, okr_id)


def render_mindnote_text(parts: list[dict[str, Any]]) -> str:
    text = "".join(str(part.get("text", "")) for part in parts if isinstance(part, dict))
    return text.strip()


def render_mindnote_nodes(nodes: list[dict[str, Any]], depth: int = 0) -> list[str]:
    lines: list[str] = []
    indent = "  " * depth
    for node in nodes:
        text = render_mindnote_text(node.get("text", []))
        if text:
            lines.append(f"{indent}- {text}")
        for child in node.get("children", []):
            lines.extend(render_mindnote_nodes([child], depth + 1))
    return lines


def read_mindnote(
    session: requests.Session,
    url: str,
    csrf_token: str,
) -> tuple[str, str, str, Counter[str]]:
    html = fetch_html(session, url, csrf_token)
    payload = json.loads(extract_balanced_object(html, "clientVars: Object("))
    title = str(payload["data"].get("title", "")).strip() or Path(urlparse(url).path).name
    nodes = payload["data"]["collab_client_vars"].get("nodes", [])
    lines = [f"# {title}", ""]
    lines.extend(render_mindnote_nodes(nodes))
    lines = normalize_lines(lines)
    counts = Counter({"mindnote_nodes": len(nodes)})
    token = str(payload.get("token", ""))
    return title, token, "\n".join(lines).strip() + "\n", counts


def read_bitable_wiki(
    session: requests.Session,
    source: str,
    html: str,
    csrf_token: str,
) -> tuple[str, str, str, Counter[str]]:
    origin = origin_for(source)
    wiki_info = extract_current_space_wiki(html)
    base_token = str(wiki_info.get("obj_token", ""))
    if not base_token:
        raise RuntimeError("Unable to locate bitable base token from wiki HTML.")
    client_vars_data = bitable_client_vars(session, origin, base_token, csrf_token, source)
    lines, counts = render_bitable_as_tsv(client_vars_data, base_token)
    title = str(
        decode_gzip_json(client_vars_data["oldSchema"]["gzipSchema"]).get("base", {}).get("name", "")
    ).strip() or base_token
    content_lines = [f"# {title}", "", f"[bitable token={base_token}]"]
    content_lines.extend(lines)
    return title, base_token, "\n".join(normalize_lines(content_lines)).strip() + "\n", counts


def detect_remote_kind(url: str) -> str:
    path = urlparse(url).path
    if "/okr/user/" in path:
        return "okr"
    if "/wiki/" in path:
        return "wiki"
    if "/docx/" in path:
        return "docx"
    if "/mindnotes/" in path:
        return "mindnote"
    return "remote"


def read_remote(
    session: requests.Session,
    source: str,
    space_api: str,
    csrf_token: str,
    expand_sheets: bool,
    *,
    download_images: bool = False,
    output_root: Path | None = None,
    asset_group: str = "",
    assets_out: list[dict[str, Any]] | None = None,
    warnings_out: list[str] | None = None,
) -> tuple[str, str, str, str, Counter[str]]:
    kind = detect_remote_kind(source)
    if kind == "okr":
        title, token, body, counts = read_okr(session, source)
        return kind, title, token, body, counts
    if kind == "mindnote":
        title, token, body, counts = read_mindnote(session, source, csrf_token)
        return kind, title, token, body, counts

    html = fetch_html(session, source, csrf_token)
    if kind == "wiki" and "window.wiki_suite_type = 'bitable'" in html:
        title, token, body, counts = read_bitable_wiki(session, source, html, csrf_token)
        return "wiki_bitable", title, token, body, counts
    origin = origin_for(source)
    token = extract_doc_token(source, html)
    data = client_vars(session, space_api, token, origin, csrf_token)
    sheet_cache: dict[str, list[str]] = {}

    def sheet_expander(sheet_block_token: str) -> list[str]:
        if sheet_block_token in sheet_cache:
            return sheet_cache[sheet_block_token]
        try:
            lines = render_embedded_sheet_as_tsv(session, origin, token, sheet_block_token, csrf_token)
        except Exception as exc:
            lines = [f"[sheet-error {exc}]", ""]
        sheet_cache[sheet_block_token] = lines
        return lines

    image_writer: ImageAssetWriter | None = None
    if download_images:
        if output_root is None:
            raise ValueError("download_images requires output_root.")
        image_writer = ImageAssetWriter(
            session=session,
            origin=origin,
            referer=source,
            headers=common_headers(origin, csrf_token, source),
            document_token=token,
            output_root=output_root,
            asset_group=asset_group or kind,
        )

    conversion = convert_docx_client_vars(
        data,
        token,
        ConversionOptions(
            expand_sheet=sheet_expander if expand_sheets else None,
            resolve_image=image_writer.resolve if image_writer is not None else None,
        ),
    )
    body = conversion.markdown
    counts = conversion.counts
    if expand_sheets:
        counts["sheet_expanded"] = len(sheet_cache)
    root = data.get("block_map", {}).get(token, {})
    root_data = root.get("data", root) if isinstance(root, dict) else {}
    title = extract_text(root_data) or token if isinstance(root_data, dict) else token
    if assets_out is not None:
        assets_out.extend(conversion.assets)
    if warnings_out is not None:
        warnings_out.extend(conversion.warnings)
    return kind, title, token, body, counts


def read_local(source: str) -> tuple[str, str]:
    path = Path(source).expanduser()
    if not path.exists():
        raise FileNotFoundError(f"Local file not found: {path}")
    return path.name, path.read_text(encoding="utf-8")


def read_sources(
    sources: list[str],
    *,
    cookies_path: Path = Path(DEFAULT_COOKIES),
    space_api: str = DEFAULT_SPACE_API,
    expand_sheets: bool = False,
    download_images: bool = False,
    output_root: Path | None = None,
) -> list[dict[str, Any]]:
    if download_images and output_root is None:
        raise ValueError("download_images requires output_root.")
    remote_sources = [source for source in sources if is_remote(source)]
    session: requests.Session | None = None
    csrf_token = ""
    if remote_sources:
        cookies = load_cookie_objects(cookies_path.expanduser())
        session = build_session(cookies)
        if any(detect_remote_kind(source) != "okr" for source in remote_sources):
            csrf_token = csrf_from(cookies)

    results: list[dict[str, Any]] = []
    for index, source in enumerate(sources, start=1):
        if is_remote(source):
            assert session is not None
            kind_hint = detect_remote_kind(source)
            assets: list[dict[str, Any]] = []
            warnings: list[str] = []
            kind, title, token, content, counts = read_remote(
                session,
                source,
                space_api,
                csrf_token,
                expand_sheets,
                download_images=download_images,
                output_root=output_root,
                asset_group=f"{kind_hint}_{index}",
                assets_out=assets,
                warnings_out=warnings,
            )
        else:
            title, content = read_local(source)
            kind, token, counts = "local_markdown", "", {}
            assets, warnings = [], []
        results.append(
            {
                "source": source,
                "kind": kind,
                "title": title,
                "token": token,
                "content": content,
                "counts": counts,
                "assets": assets,
                "warnings": warnings,
            }
        )
    return results


def write_outputs(results: list[dict[str, object]], out_dir: Path) -> dict[str, dict[str, object]]:
    out_dir.mkdir(parents=True, exist_ok=True)
    manifest: dict[str, dict[str, object]] = {}
    used_stems: set[str] = set()
    for index, result in enumerate(results, start=1):
        stem = f"{result['kind']}_{index}"
        path = out_dir / f"{output_file_stem(result, stem, used_stems)}.md"
        path.write_text(str(result["content"]), encoding="utf-8")
        result["file"] = str(path)
        manifest[stem] = {
            "title": result["title"],
            "token": result["token"],
            "kind": result["kind"],
            "counts": result["counts"],
            "file": str(path),
            "source": result["source"],
            "assets": result.get("assets", []),
            "warnings": result.get("warnings", []),
        }
    (out_dir / "manifest.json").write_text(
        json.dumps(manifest, ensure_ascii=False, indent=2),
        encoding="utf-8",
    )
    return manifest


def cleanup_outputs(manifest: dict[str, dict[str, object]], out_dir: Path) -> None:
    root = out_dir.resolve()
    generated_paths: list[Path] = []
    generated_dirs: set[Path] = set()
    for item in manifest.values():
        file_path = generated_path(out_dir, root, item.get("file"))
        if file_path is not None:
            generated_paths.append(file_path)
        assets = item.get("assets", [])
        if not isinstance(assets, list):
            continue
        for asset in assets:
            if not isinstance(asset, dict):
                continue
            asset_path = generated_path(out_dir, root, asset.get("path"))
            if asset_path is None:
                continue
            generated_paths.append(asset_path)
            generated_dirs.add(asset_path.parent)

    for path in generated_paths:
        with contextlib.suppress(FileNotFoundError):
            path.unlink()
    with contextlib.suppress(FileNotFoundError):
        (out_dir / "manifest.json").unlink()
    generated_dirs.add(out_dir / "assets")
    for directory in sorted(generated_dirs, key=lambda path: len(path.parts), reverse=True):
        with contextlib.suppress(OSError):
            directory.rmdir()
    with contextlib.suppress(OSError):
        out_dir.rmdir()


def generated_path(out_dir: Path, root: Path, raw_path: object) -> Path | None:
    if not raw_path:
        return None
    path = Path(str(raw_path))
    candidate = path if path.is_absolute() else out_dir / path
    resolved = candidate.resolve()
    try:
        resolved.relative_to(root)
    except ValueError:
        return None
    return resolved


def slugify(value: str) -> str:
    text = re.sub(r"[^a-zA-Z0-9]+", "-", value.strip().lower()).strip("-")
    return text or "doc"


def output_file_stem(result: dict[str, object], fallback: str, used_stems: set[str]) -> str:
    base = slugify(fallback)
    if result.get("kind") == "local_markdown":
        source = Path(str(result.get("source", ""))).expanduser()
        base = slugify(source.stem or str(result.get("title", "")) or fallback)

    candidate = base
    suffix = 2
    while candidate in used_stems:
        candidate = f"{base}-{suffix}"
        suffix += 1
    used_stems.add(candidate)
    return candidate


def inspect_source(source: str) -> dict[str, object]:
    if source.startswith(("http://", "https://")):
        return inspect_remote_source(source)
    return inspect_local_source(source)


def inspect_local_source(source: str) -> dict[str, object]:
    path = Path(source).expanduser()
    if not path.exists():
        raise FileNotFoundError(f"local file not found: {path}")
    return {
        "ok": True,
        "source": source,
        "remote": False,
        "kind": "local_markdown",
        "path": str(path),
        "exists": True,
        "readable": os.access(path, os.R_OK),
        "sizeBytes": path.stat().st_size,
        "suffix": path.suffix,
    }


def inspect_remote_source(source: str) -> dict[str, object]:
    parsed = urlparse(source)
    path = parsed.path
    path_type = "remote"
    token = ""
    owner_id = ""
    kind = "remote"
    route = "remote_read"

    if "/okr/user/" in path:
        path_type = "okr"
        owner_match = re.search(r"/okr/user/([^/?#]+)", path)
        if owner_match:
            owner_id = owner_match.group(1)
        query = parse_qs(parsed.query)
        token = (query.get("okrId") or query.get("okr_id") or [""])[0]
    else:
        for candidate in ("docx", "wiki", "mindnotes"):
            match = re.search(rf"/{candidate}/([^/?#]+)", path)
            if match:
                path_type = candidate
                token = match.group(1)
                break

    if path_type == "docx":
        kind = "docx"
        route = "docx_client_vars"
    elif path_type == "wiki":
        kind = "wiki"
        route = "wiki_resolve_then_read"
    elif path_type == "mindnotes":
        kind = "mindnote"
        route = "mindnote_client_vars"
    elif path_type == "okr":
        kind = "okr"
        route = "okr_detail"

    return {
        "ok": True,
        "sourceRef": redacted_remote_source(
            parsed.path,
            parsed.netloc,
            parsed.query,
            [owner_id, token],
        ),
        "remote": True,
        "kind": kind,
        "host": parsed.netloc,
        "pathType": path_type,
        "tokenPrefix": token[:3],
        "tokenLength": len(token),
        "route": route,
    }


def redacted_remote_source(
    path: str,
    netloc: str,
    query: str,
    tokens: list[str],
) -> str:
    redacted_path = path
    redacted_query = query
    for token in tokens:
        if not token:
            continue
        redacted_path = redacted_path.replace(token, "<redacted>")
        redacted_query = redacted_query.replace(token, "<redacted>")
    suffix = f"?{redacted_query}" if redacted_query else ""
    return f"https://{netloc}{redacted_path}{suffix}"
