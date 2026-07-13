from __future__ import annotations

from collections import Counter
import json
from pathlib import Path
from typing import Any
from urllib.parse import parse_qs, urlencode, urlparse

import requests

from ixf_toolbox.core.cookies import DEFAULT_COOKIES, load_cookie_objects


USER_AGENT = (
    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
    "AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0 Safari/537.36"
)
DEFAULT_OKR_CSRF_URL = "https://www.xfchat.iflytek.com/lgw/csrf_token"


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


def origin_for(url: str) -> str:
    parsed = urlparse(url)
    return f"{parsed.scheme}://{parsed.netloc}"


def detect_okr_url(source: str) -> bool:
    parsed = urlparse(source)
    return parsed.scheme in {"http", "https"} and "/okr/user/" in parsed.path


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
    response = session.get(DEFAULT_OKR_CSRF_URL, timeout=30)
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
    period = str(
        detail.get("name") or detail.get("period_name") or detail.get("periodName") or ""
    ).strip()
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


def read_okr_url(
    source: str,
    *,
    cookies_path: Path = Path(DEFAULT_COOKIES),
) -> dict[str, object]:
    if not detect_okr_url(source):
        raise ValueError("source is not an OKR page URL.")
    cookies = load_cookie_objects(cookies_path.expanduser())
    session = build_session(cookies)
    title, token, content, counts = read_okr(session, source)
    return {
        "source": source,
        "kind": "okr",
        "title": title,
        "token": token,
        "content": content,
        "counts": counts,
    }
