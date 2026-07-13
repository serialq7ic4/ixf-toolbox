from __future__ import annotations

import hashlib
import json
import platform
import random
import re
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Callable
from urllib.parse import parse_qs, urlencode, urlparse, urlunparse

import requests

from ixf_toolbox.core.cookies import DEFAULT_COOKIES, export_cookie_session, load_cookie_objects


DEFAULT_CSRF_URL = "https://www.xfchat.iflytek.com/lgw/csrf_token"
TOKEN_SALT = "lk_anonymous_id"
USER_AGENT = (
    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
    "AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0 Safari/537.36"
)

LGW_TERMINAL_WEB = "2"
LGW_OS_UNKNOWN = "0"
LGW_OS_WINDOWS = "1"
LGW_OS_LINUX = "2"
LGW_OS_MAC = "3"


class NeedVersionRefresh(Exception):
    pass


class DraftVersionCache:
    def __init__(self) -> None:
        self.value: str | None = None

    def get(self) -> str | None:
        return self.value

    def set(self, version: Any) -> None:
        if version:
            self.value = str(version)

    def clear(self) -> None:
        self.value = None


@dataclass(frozen=True)
class ObjectiveSpec:
    objective: str
    krs: list[str]


@dataclass(frozen=True)
class OkrWriteConfig:
    url: str
    input_path: Path
    cookies_path: Path = Path(DEFAULT_COOKIES)
    base_url: str = ""
    objective_index: int | None = None
    prune: bool = False
    apply: bool = False


def random_token() -> str:
    raw = f"{int(time.time() * 1000)}@{random.random()}{TOKEN_SALT}"
    return hashlib.md5(raw.encode(), usedforsecurity=False).hexdigest()


def okr_id_from_url_or_none(url: str) -> str | None:
    query = parse_qs(urlparse(url).query)
    okr_id = (query.get("okrId") or query.get("okr_id") or [""])[0]
    return okr_id or None


def url_with_okr_id(url: str, okr_id: str) -> str:
    parsed = urlparse(url)
    query = parse_qs(parsed.query)
    query["okrId"] = [okr_id]
    return urlunparse(parsed._replace(query=urlencode(query, doseq=True)))


def base_url_from_url(url: str) -> str:
    parsed = urlparse(url)
    if parsed.scheme not in {"http", "https"} or not parsed.netloc:
        raise ValueError("OKR URL must be an absolute HTTP(S) URL.")
    return f"{parsed.scheme}://{parsed.netloc}"


def ensure_cookie_file(cookie_path: Path) -> None:
    if cookie_path.expanduser().exists():
        return
    export_cookie_session(provider="auto", output=cookie_path.expanduser())


def build_session(cookies: list[dict[str, Any]], base_url: str, okr_url: str) -> requests.Session:
    session = requests.Session()
    session.headers.update(
        {
            "User-Agent": USER_AGENT,
            "Accept": "application/json,text/plain,*/*",
            "Accept-Language": "zh-CN,zh;q=0.9,en;q=0.8",
            "Origin": base_url,
            "Referer": okr_url,
            "Sec-Fetch-Site": "same-origin",
            "Sec-Fetch-Mode": "cors",
            "Sec-Fetch-Dest": "empty",
        }
    )
    for cookie in cookies:
        if cookie.get("name") and cookie.get("value") is not None:
            session.cookies.set(
                str(cookie["name"]),
                str(cookie["value"]),
                domain=cookie.get("domain"),
                path=cookie.get("path", "/"),
            )
    return session


def lgw_os_type() -> str:
    system = platform.system().lower()
    if system == "darwin":
        return LGW_OS_MAC
    if system == "windows":
        return LGW_OS_WINDOWS
    if system == "linux":
        return LGW_OS_LINUX
    return LGW_OS_UNKNOWN


def lgw_headers(token: str) -> dict[str, str]:
    return {
        "x-lgw-csrf-token": token,
        "x-lgw-terminal-type": LGW_TERMINAL_WEB,
        "x-lgw-os-type": lgw_os_type(),
    }


def parse_preloaded_okr(html: str) -> dict[str, Any]:
    match = re.search(r'window\.preloadedOKR=JSON\.parse\("(.*?)"\)', html, re.S)
    if not match:
        raise RuntimeError("preloadedOKR not found in OKR page HTML.")
    return json.loads(json.loads(f'"{match.group(1)}"'))


def parse_maybe_json(value: Any) -> Any:
    if isinstance(value, str):
        try:
            return json.loads(value)
        except json.JSONDecodeError:
            return value
    return value


def text_from_v2(value: Any) -> str:
    value = parse_maybe_json(value)
    if not value:
        return ""
    if isinstance(value, str):
        return value.strip()
    if isinstance(value, dict):
        ops = ((value.get("0") or {}).get("ops") or [])
        return "".join(str(op.get("insert", "")) for op in ops).strip()
    return str(value).strip()


def text_from_content(value: Any) -> str:
    value = parse_maybe_json(value)
    if not value:
        return ""
    if isinstance(value, str):
        return value.strip()
    if isinstance(value, dict):
        return "\n".join(str(block.get("text") or "") for block in value.get("blocks", [])).strip()
    return str(value).strip()


def objective_text(objective: dict[str, Any]) -> str:
    return (
        text_from_v2(objective.get("content_v2") or objective.get("contentV2"))
        or text_from_content(objective.get("content"))
        or text_from_content(objective.get("name"))
    )


def kr_text(kr: dict[str, Any]) -> str:
    return (
        text_from_content(kr.get("content"))
        or text_from_v2(kr.get("content_v2") or kr.get("contentV2"))
        or str(kr.get("name") or "").strip()
    )


def text_delta(text: str) -> list[dict[str, str]]:
    return [{"insert": f"{text}\n"}]


def delta_doc(text: str) -> dict[str, Any]:
    return {
        "0": {
            "ops": text_delta(text),
            "zoneId": "0",
            "zoneType": "Z",
        }
    }


def delta_doc_json(text: str) -> str:
    return json.dumps(delta_doc(text), ensure_ascii=False, separators=(",", ":"))


def delta_plain_text(delta: list[dict[str, Any]], *, default: str = "\n") -> str:
    text = "".join(str(operation.get("insert", "")) for operation in delta if isinstance(operation, dict))
    return text if text else default


def content_v2_delta(value: Any) -> list[dict[str, Any]]:
    value = parse_maybe_json(value)
    if isinstance(value, dict):
        operations = (value.get("0") or {}).get("ops")
        if isinstance(operations, list):
            return [operation for operation in operations if isinstance(operation, dict)]
    if isinstance(value, list):
        return [operation for operation in value if isinstance(operation, dict)]
    return [{"insert": "\n"}]


def editor_delta_stats(previous: list[dict[str, Any]], text: str) -> str:
    old_text = delta_plain_text(previous)
    new_text = f"{text}\n"
    if old_text == "\n":
        stats = [{"type": "insert", "offset": 0, "length": max(0, len(new_text) - 1)}]
    else:
        stats: list[dict[str, Any]] = []
        if old_text:
            stats.append({"type": "delete", "offset": 0, "length": len(old_text)})
        if new_text:
            stats.append({"type": "insert", "offset": 0, "length": len(new_text)})
    return json.dumps(stats, ensure_ascii=False, separators=(",", ":"))


def draft_v2_body(version: str, conn_uuid: str) -> dict[str, str]:
    return {
        "draft_version": version,
        "token": random_token(),
        "conn_uuid": conn_uuid,
    }


def delete_params(version: str, conn_uuid: str) -> dict[str, str]:
    return {
        "draft_version": version,
        "token": random_token(),
        "conn_uuid": conn_uuid,
    }


def get_state(session: requests.Session, okr_url: str) -> dict[str, Any]:
    response = session.get(
        okr_url,
        headers={"Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"},
        timeout=30,
    )
    response.raise_for_status()
    return parse_preloaded_okr(response.text)


def okr_id_from_state(state: dict[str, Any]) -> str | None:
    candidates = [
        state.get("okrId"),
        state.get("okr_id"),
        state.get("id"),
        (state.get("okr") or {}).get("id") if isinstance(state.get("okr"), dict) else None,
        (state.get("okr_detail_data") or {}).get("id")
        if isinstance(state.get("okr_detail_data"), dict)
        else None,
    ]
    for candidate in candidates:
        if candidate:
            return str(candidate)
    return None


def resolve_okr_id(session: requests.Session, okr_url: str) -> tuple[str, str]:
    okr_id = okr_id_from_url_or_none(okr_url)
    if okr_id:
        return okr_id, okr_url
    state = get_state(session, okr_url)
    okr_id = okr_id_from_state(state)
    if not okr_id:
        raise RuntimeError("Unable to determine the OKR id.")
    return okr_id, url_with_okr_id(okr_url, okr_id)


def refresh_lgw_csrf_token(
    session: requests.Session,
    okr_url: str,
    *,
    read_page_first: bool = True,
) -> str:
    if read_page_first:
        get_state(session, okr_url)
    base_url = base_url_from_url(okr_url)
    session.get(
        f"{base_url}/okrx/api/self/",
        headers={
            "Accept": "application/json,text/plain,*/*",
            "Agw-Js-Conv": "1",
            "x-requested-with": "XMLHttpRequest",
            "x-is-cacheable": "true",
            "Referer": okr_url,
        },
        timeout=20,
    )
    response = session.get(
        DEFAULT_CSRF_URL,
        headers={
            "Accept": "application/json,text/plain,*/*",
            "Referer": okr_url,
        },
        timeout=20,
    )
    response.raise_for_status()
    token = str(
        session.cookies.get("lgw_csrf_token", domain=".xfchat.iflytek.com")
        or session.cookies.get("lgw_csrf_token")
        or ""
    )
    if not token:
        raise RuntimeError("Unable to obtain the OKR CSRF token.")
    return token


def get_detail(
    session: requests.Session,
    base_url: str,
    okr_url: str,
    okr_id: str,
) -> dict[str, Any]:
    token = refresh_lgw_csrf_token(session, okr_url, read_page_first=False)
    response = session.get(
        f"{base_url}/okrx/api/okr/owner/aggr_detail/",
        params={
            "okr_id": okr_id,
            "withoutAddVisitLog": "true",
        },
        headers={
            **lgw_headers(token),
            "Referer": okr_url,
            "Origin": base_url,
        },
        timeout=30,
    )
    response.raise_for_status()
    payload = response.json()
    if payload.get("code") not in {0, None}:
        raise RuntimeError("Could not load OKR details.")
    detail = payload.get("okr_detail_data") or payload.get("data") or {}
    if not isinstance(detail, dict):
        raise RuntimeError("OKR detail returned an unexpected payload.")
    return detail


def current_draft_version(
    session: requests.Session,
    base_url: str,
    okr_url: str,
    okr_id: str,
) -> str:
    response = session.get(
        f"{base_url}/okrx/api/okr/{okr_id}/version/",
        headers={
            "Accept": "application/json,text/plain,*/*",
            "Referer": okr_url,
            "Origin": base_url,
        },
        timeout=30,
    )
    if response.ok:
        payload = response.json()
        data = payload.get("data") or {}
        version = data.get("okr_draft_version") or data.get("okr_version")
        if version:
            return str(version)
    state = get_state(session, okr_url)
    version = state.get("version") or state.get("current_version")
    if not version:
        raise RuntimeError("Unable to determine the OKR draft version.")
    return str(version)


def okr_api(
    session: requests.Session,
    method: str,
    base_url: str,
    okr_url: str,
    path: str,
    *,
    body: dict[str, Any] | None = None,
    params: dict[str, Any] | None = None,
    allow_refresh_retry: bool = True,
) -> dict[str, Any]:
    token = refresh_lgw_csrf_token(session, okr_url)
    response = session.request(
        method,
        f"{base_url}{path}",
        json=body,
        params=params,
        headers={
            "Accept": "application/json,text/plain,*/*",
            "Content-Type": "application/json;charset=UTF-8",
            "Referer": okr_url,
            "Origin": base_url,
            **lgw_headers(token),
            "x-requested-with": "XMLHttpRequest",
            "agw-js-conv": "1",
            "okr-language": "zh-CN",
            "okr-timezone": "Asia/Shanghai",
        },
        timeout=30,
    )
    try:
        payload = response.json()
    except Exception as exc:
        raise RuntimeError(f"{method} {path} returned a non-JSON response.") from exc
    ok = (
        response.ok
        and payload.get("code") in {0, None}
        and payload.get("success", True) is not False
    )
    if ok:
        return payload
    message = str(payload.get("msg") or payload.get("message") or "")
    if allow_refresh_retry and payload.get("code") == 100001:
        raise NeedVersionRefresh(message)
    raise RuntimeError(f"{method} {path} failed with code={payload.get('code')}.")


def draft_version_from_payload(payload: dict[str, Any]) -> str | None:
    data = payload.get("data")
    candidates: list[Any] = []
    if isinstance(data, dict):
        candidates.extend(
            [
                data.get("draftVersion"),
                data.get("draft_version"),
                data.get("okrDraftVersion"),
                data.get("okr_draft_version"),
                data.get("version"),
            ]
        )
    candidates.extend(
        [
            payload.get("draftVersion"),
            payload.get("draft_version"),
            payload.get("okrDraftVersion"),
            payload.get("okr_draft_version"),
            payload.get("version"),
        ]
    )
    for candidate in candidates:
        if candidate:
            return str(candidate)
    return None


def call_with_version(
    session: requests.Session,
    method: str,
    base_url: str,
    okr_url: str,
    okr_id: str,
    path: str,
    *,
    make_body: Callable[[str, str], dict[str, Any]] | None = None,
    make_params: Callable[[str, str], dict[str, Any]] | None = None,
    conn_uuid: str,
    version_cache: DraftVersionCache | None = None,
) -> dict[str, Any]:
    for _ in range(3):
        version = (
            version_cache.get()
            if version_cache and version_cache.get()
            else current_draft_version(session, base_url, okr_url, okr_id)
        )
        try:
            payload = okr_api(
                session,
                method,
                base_url,
                okr_url,
                path,
                body=make_body(version, conn_uuid) if make_body else None,
                params=make_params(version, conn_uuid) if make_params else None,
            )
            if version_cache:
                version_cache.set(draft_version_from_payload(payload))
            return payload
        except NeedVersionRefresh:
            if version_cache:
                version_cache.clear()
            time.sleep(0.2)
    version = current_draft_version(session, base_url, okr_url, okr_id)
    payload = okr_api(
        session,
        method,
        base_url,
        okr_url,
        path,
        body=make_body(version, conn_uuid) if make_body else None,
        params=make_params(version, conn_uuid) if make_params else None,
        allow_refresh_retry=False,
    )
    if version_cache:
        version_cache.set(draft_version_from_payload(payload))
    return payload


def parse_specs(path: Path) -> list[ObjectiveSpec]:
    payload = json.loads(path.expanduser().read_text(encoding="utf-8"))
    raw_items = payload.get("objectives") if isinstance(payload, dict) else payload
    if not isinstance(raw_items, list):
        raise ValueError("OKR input must contain an objectives list.")
    specs: list[ObjectiveSpec] = []
    for index, item in enumerate(raw_items, start=1):
        if not isinstance(item, dict):
            raise ValueError(f"Objective {index} must be an object.")
        objective = str(item.get("objective") or item.get("o") or "").strip()
        krs = [str(kr).strip() for kr in item.get("krs", []) if str(kr).strip()]
        if not objective:
            raise ValueError(f"Objective {index} is blank.")
        if not krs:
            raise ValueError(f"Objective {index} has no KRs.")
        if len(krs) > 4:
            raise ValueError(f"Objective {index} has {len(krs)} KRs; keep OKR scope realistic.")
        specs.append(ObjectiveSpec(objective=objective, krs=krs))
    if not specs:
        raise ValueError("OKR input contains no objectives.")
    return specs


def objective_list(detail: dict[str, Any]) -> list[dict[str, Any]]:
    items = detail.get("objective_list", []) or detail.get("objectiveList", [])
    return [item for item in items if isinstance(item, dict)]


def summarize(detail: dict[str, Any]) -> list[dict[str, Any]]:
    return [
        {
            "id": str(objective.get("id")),
            "objective": objective_text(objective),
            "krs": [
                {"id": str(kr.get("id")), "text": kr_text(kr)}
                for kr in (objective.get("kr_list", []) or objective.get("krList", []) or [])
                if isinstance(kr, dict)
            ],
        }
        for objective in objective_list(detail)
    ]


def public_summary(items: list[dict[str, Any]]) -> list[dict[str, Any]]:
    return [{"objective": item["objective"], "krs": [kr["text"] for kr in item["krs"]]} for item in items]


def find_objective_raw(detail: dict[str, Any], objective_id: str) -> dict[str, Any] | None:
    for objective in objective_list(detail):
        if str(objective.get("id")) == str(objective_id):
            return objective
    return None


def find_kr_raw(detail: dict[str, Any], kr_id: str) -> dict[str, Any] | None:
    for objective in objective_list(detail):
        kr_items = objective.get("kr_list", []) or objective.get("krList", [])
        for kr in kr_items:
            if isinstance(kr, dict) and str(kr.get("id")) == str(kr_id):
                return kr
    return None


def objective_at_index(current: list[dict[str, Any]], index: int) -> dict[str, Any] | None:
    if index < 1:
        raise ValueError("--objective-index must be at least 1.")
    if index <= len(current):
        return current[index - 1]
    if index == len(current) + 1:
        return None
    raise ValueError(
        f"--objective-index {index} cannot skip positions; current objective count is {len(current)}."
    )


def non_target_objectives(state: list[dict[str, Any]], index: int) -> list[dict[str, Any]]:
    return [item for position, item in enumerate(state, start=1) if position != index]


def plan_changes(
    current: list[dict[str, Any]],
    specs: list[ObjectiveSpec],
    prune: bool,
    objective_index: int | None,
) -> list[str]:
    if objective_index is not None:
        if len(specs) != 1:
            raise ValueError("--objective-index requires exactly one Objective.")
        existing = objective_at_index(current, objective_index)
        action = "update" if existing else "create"
        return [
            f"target objective index: O{objective_index}",
            f"{action} objective: {specs[0].objective}",
            *[f"set KR: {text}" for text in specs[0].krs],
            "preserve non-target objectives",
        ]
    actions: list[str] = []
    current_by_text = {item["objective"]: item for item in current}
    for spec in specs:
        existing = current_by_text.get(spec.objective)
        actions.append(f"{'update' if existing else 'create'} objective: {spec.objective}")
        actions.extend(f"ensure KR: {text}" for text in spec.krs)
    if prune:
        actions.append("delete objectives and KRs not present in the input")
    return actions


def update_objective_text(
    session: requests.Session,
    base_url: str,
    okr_url: str,
    okr_id: str,
    objective_id: str,
    text: str,
    conn_uuid: str,
    version_cache: DraftVersionCache | None = None,
) -> None:
    raw = find_objective_raw(get_detail(session, base_url, okr_url, okr_id), objective_id)
    previous_delta = content_v2_delta((raw or {}).get("content_v2") or (raw or {}).get("contentV2"))

    def make_body(version: str, conn: str) -> dict[str, Any]:
        payload = draft_v2_body(version, conn)
        payload.update({"changesets": editor_delta_stats(previous_delta, text), "name": delta_doc_json(text)})
        return payload

    call_with_version(
        session,
        "PUT",
        base_url,
        okr_url,
        okr_id,
        f"/okrx/api/draft_v2/objective/{objective_id}/",
        make_body=make_body,
        conn_uuid=conn_uuid,
        version_cache=version_cache,
    )


def create_objective(
    session: requests.Session,
    base_url: str,
    okr_url: str,
    okr_id: str,
    text: str,
    conn_uuid: str,
    version_cache: DraftVersionCache | None = None,
) -> str:
    def make_body(version: str, conn: str) -> dict[str, Any]:
        payload = draft_v2_body(version, conn)
        payload.update({"changesets": "[]", "name": delta_doc_json(""), "okr_id": okr_id})
        return payload

    response = call_with_version(
        session,
        "POST",
        base_url,
        okr_url,
        okr_id,
        "/okrx/api/draft_v2/objective/",
        make_body=make_body,
        conn_uuid=conn_uuid,
        version_cache=version_cache,
    )
    data = response.get("data") or {}
    objective_id = str(data.get("objectiveId") or data.get("objective_id") or "")
    if not objective_id:
        raise RuntimeError("Objective creation did not return an identifier.")
    update_objective_text(session, base_url, okr_url, okr_id, objective_id, text, conn_uuid, version_cache)
    return objective_id


def update_kr_text(
    session: requests.Session,
    base_url: str,
    okr_url: str,
    okr_id: str,
    kr_id: str,
    text: str,
    conn_uuid: str,
    version_cache: DraftVersionCache | None = None,
) -> None:
    raw = find_kr_raw(get_detail(session, base_url, okr_url, okr_id), kr_id)
    previous_delta = content_v2_delta((raw or {}).get("content_v2") or (raw or {}).get("contentV2"))

    def make_body(version: str, conn: str) -> dict[str, Any]:
        payload = draft_v2_body(version, conn)
        payload.update({"changesets": editor_delta_stats(previous_delta, text), "content": delta_doc_json(text)})
        return payload

    call_with_version(
        session,
        "PUT",
        base_url,
        okr_url,
        okr_id,
        f"/okrx/api/draft_v2/kr/{kr_id}/",
        make_body=make_body,
        conn_uuid=conn_uuid,
        version_cache=version_cache,
    )


def create_kr(
    session: requests.Session,
    base_url: str,
    okr_url: str,
    okr_id: str,
    objective_id: str,
    text: str,
    conn_uuid: str,
    version_cache: DraftVersionCache | None = None,
) -> str:
    def make_body(version: str, conn: str) -> dict[str, Any]:
        payload = draft_v2_body(version, conn)
        payload.update({"changesets": "[]", "content": delta_doc_json(""), "objective_id": objective_id})
        return payload

    response = call_with_version(
        session,
        "POST",
        base_url,
        okr_url,
        okr_id,
        "/okrx/api/draft_v2/kr/",
        make_body=make_body,
        conn_uuid=conn_uuid,
        version_cache=version_cache,
    )
    data = response.get("data") or {}
    kr_id = str(data.get("krId") or data.get("kr_id") or "")
    if not kr_id:
        raise RuntimeError("KR creation did not return an identifier.")
    update_kr_text(session, base_url, okr_url, okr_id, kr_id, text, conn_uuid, version_cache)
    return kr_id


def delete_kr(
    session: requests.Session,
    base_url: str,
    okr_url: str,
    okr_id: str,
    kr_id: str,
    conn_uuid: str,
    version_cache: DraftVersionCache | None = None,
) -> None:
    call_with_version(
        session,
        "DELETE",
        base_url,
        okr_url,
        okr_id,
        f"/okrx/api/draft_v2/kr/{kr_id}/",
        make_params=lambda version, conn: delete_params(version, conn),
        conn_uuid=conn_uuid,
        version_cache=version_cache,
    )


def delete_objective(
    session: requests.Session,
    base_url: str,
    okr_url: str,
    okr_id: str,
    objective_id: str,
    conn_uuid: str,
    version_cache: DraftVersionCache | None = None,
) -> None:
    call_with_version(
        session,
        "DELETE",
        base_url,
        okr_url,
        okr_id,
        f"/okrx/api/draft_v2/objective/{objective_id}/",
        make_params=lambda version, conn: delete_params(version, conn),
        conn_uuid=conn_uuid,
        version_cache=version_cache,
    )


def enable_objective_draft(
    session: requests.Session,
    base_url: str,
    okr_url: str,
    okr_id: str,
    objective_id: str,
    conn_uuid: str,
    version_cache: DraftVersionCache | None = None,
) -> None:
    call_with_version(
        session,
        "POST",
        base_url,
        okr_url,
        okr_id,
        f"/okrx/api/draft_v2/enable/{objective_id}/",
        make_body=lambda version, conn: draft_v2_body(version, conn),
        conn_uuid=conn_uuid,
        version_cache=version_cache,
    )


def publish_objective(
    session: requests.Session,
    base_url: str,
    okr_url: str,
    okr_id: str,
    objective_id: str,
    conn_uuid: str,
    version_cache: DraftVersionCache | None = None,
    *,
    delete_kr_ids: list[str] | None = None,
    auto_notify: bool = False,
) -> None:
    def make_body(version: str, conn: str) -> dict[str, Any]:
        payload = draft_v2_body(version, conn)
        payload.update({"need_delete_kr_ids": delete_kr_ids or [], "auto_notify": auto_notify})
        return payload

    call_with_version(
        session,
        "POST",
        base_url,
        okr_url,
        okr_id,
        f"/okrx/api/draft_v2/publish/{objective_id}/",
        make_body=make_body,
        conn_uuid=conn_uuid,
        version_cache=version_cache,
    )


def order_krs(
    session: requests.Session,
    base_url: str,
    okr_url: str,
    okr_id: str,
    objective_id: str,
    kr_ids: list[str],
    conn_uuid: str,
    version_cache: DraftVersionCache | None = None,
) -> None:
    def make_body(version: str, conn: str) -> dict[str, Any]:
        payload = draft_v2_body(version, conn)
        payload.update({"krIds": kr_ids, "objectiveId": objective_id})
        return payload

    call_with_version(
        session,
        "POST",
        base_url,
        okr_url,
        okr_id,
        "/okrx/api/draft_v2/kr/pos/",
        make_body=make_body,
        conn_uuid=conn_uuid,
        version_cache=version_cache,
    )


def replace_target_krs(
    *,
    session: requests.Session,
    base_url: str,
    okr_url: str,
    okr_id: str,
    objective_id: str,
    target_krs: list[str],
    conn_uuid: str,
    version_cache: DraftVersionCache,
) -> list[str]:
    detail = get_detail(session, base_url, okr_url, okr_id)
    raw = find_objective_raw(detail, objective_id)
    existing = (raw or {}).get("kr_list", []) or (raw or {}).get("krList", []) or []
    deleted_ids = [str(kr.get("id")) for kr in existing if isinstance(kr, dict) and kr.get("id")]
    for kr_id in deleted_ids:
        delete_kr(session, base_url, okr_url, okr_id, kr_id, conn_uuid, version_cache)
    for text in target_krs:
        create_kr(session, base_url, okr_url, okr_id, objective_id, text, conn_uuid, version_cache)
    return deleted_ids


def verify_target_and_preserved_neighbors(
    *,
    session: requests.Session,
    base_url: str,
    okr_url: str,
    okr_id: str,
    index: int,
    spec: ObjectiveSpec,
    before_state: list[dict[str, Any]],
) -> dict[str, Any]:
    final = summarize(get_detail(session, base_url, okr_url, okr_id))
    if index > len(final):
        raise RuntimeError(f"O{index} was not found after writing.")
    actual = final[index - 1]
    if actual["objective"] != spec.objective:
        raise RuntimeError(f"O{index} text did not match after writing.")
    if [kr["text"] for kr in actual["krs"]] != spec.krs:
        raise RuntimeError(f"O{index} KR content did not match after writing.")
    if non_target_objectives(before_state, index) != non_target_objectives(final, index):
        raise RuntimeError("A non-target Objective changed during the write.")
    return {"objective": actual["objective"], "krs": [{"text": kr["text"]} for kr in actual["krs"]]}


def update_existing_target(
    *,
    session: requests.Session,
    base_url: str,
    okr_url: str,
    okr_id: str,
    existing: dict[str, Any],
    spec: ObjectiveSpec,
    index: int,
    before_state: list[dict[str, Any]],
    conn_uuid: str,
) -> dict[str, Any]:
    cache = DraftVersionCache()
    objective_id = str(existing["id"])
    enable_objective_draft(session, base_url, okr_url, okr_id, objective_id, conn_uuid, cache)
    update_objective_text(session, base_url, okr_url, okr_id, objective_id, spec.objective, conn_uuid, cache)
    deleted_ids = replace_target_krs(
        session=session,
        base_url=base_url,
        okr_url=okr_url,
        okr_id=okr_id,
        objective_id=objective_id,
        target_krs=spec.krs,
        conn_uuid=conn_uuid,
        version_cache=cache,
    )
    publish_objective(
        session,
        base_url,
        okr_url,
        okr_id,
        objective_id,
        conn_uuid,
        cache,
        delete_kr_ids=deleted_ids,
    )
    return verify_target_and_preserved_neighbors(
        session=session,
        base_url=base_url,
        okr_url=okr_url,
        okr_id=okr_id,
        index=index,
        spec=spec,
        before_state=before_state,
    )


def create_target_at_index(
    *,
    session: requests.Session,
    base_url: str,
    okr_url: str,
    okr_id: str,
    spec: ObjectiveSpec,
    index: int,
    before_state: list[dict[str, Any]],
    conn_uuid: str,
) -> dict[str, Any]:
    cache = DraftVersionCache()
    objective_id = create_objective(session, base_url, okr_url, okr_id, spec.objective, conn_uuid, cache)
    for text in spec.krs:
        create_kr(session, base_url, okr_url, okr_id, objective_id, text, conn_uuid, cache)
    publish_objective(session, base_url, okr_url, okr_id, objective_id, conn_uuid, cache)
    return verify_target_and_preserved_neighbors(
        session=session,
        base_url=base_url,
        okr_url=okr_url,
        okr_id=okr_id,
        index=index,
        spec=spec,
        before_state=before_state,
    )


def write_target_objective_index(
    *,
    session: requests.Session,
    base_url: str,
    okr_url: str,
    okr_id: str,
    spec: ObjectiveSpec,
    index: int,
    conn_uuid: str,
) -> dict[str, Any]:
    state = summarize(get_detail(session, base_url, okr_url, okr_id))
    existing = objective_at_index(state, index)
    if existing is not None:
        return update_existing_target(
            session=session,
            base_url=base_url,
            okr_url=okr_url,
            okr_id=okr_id,
            existing=existing,
            spec=spec,
            index=index,
            before_state=state,
            conn_uuid=conn_uuid,
        )
    return create_target_at_index(
        session=session,
        base_url=base_url,
        okr_url=okr_url,
        okr_id=okr_id,
        spec=spec,
        index=index,
        before_state=state,
        conn_uuid=conn_uuid,
    )


def delete_published_objective(
    session: requests.Session,
    base_url: str,
    okr_url: str,
    okr_id: str,
    objective_id: str,
    conn_uuid: str,
) -> None:
    cache = DraftVersionCache()
    enable_objective_draft(session, base_url, okr_url, okr_id, objective_id, conn_uuid, cache)
    delete_objective(session, base_url, okr_url, okr_id, objective_id, conn_uuid, cache)


def write_specs(
    *,
    session: requests.Session,
    base_url: str,
    okr_url: str,
    okr_id: str,
    specs: list[ObjectiveSpec],
    prune: bool,
    conn_uuid: str,
) -> list[dict[str, Any]]:
    state = summarize(get_detail(session, base_url, okr_url, okr_id))
    target_texts = {spec.objective for spec in specs}
    if prune:
        for item in list(state):
            if item["objective"] not in target_texts:
                delete_published_objective(session, base_url, okr_url, okr_id, str(item["id"]), conn_uuid)
        state = summarize(get_detail(session, base_url, okr_url, okr_id))

    for spec in specs:
        state = summarize(get_detail(session, base_url, okr_url, okr_id))
        existing = next((item for item in state if item["objective"] == spec.objective), None)
        if existing is None:
            cache = DraftVersionCache()
            objective_id = create_objective(session, base_url, okr_url, okr_id, spec.objective, conn_uuid, cache)
            for text in spec.krs:
                create_kr(session, base_url, okr_url, okr_id, objective_id, text, conn_uuid, cache)
            publish_objective(session, base_url, okr_url, okr_id, objective_id, conn_uuid, cache)
            continue

        existing_texts = [kr["text"] for kr in existing["krs"]]
        if existing_texts == spec.krs:
            continue
        cache = DraftVersionCache()
        objective_id = str(existing["id"])
        enable_objective_draft(session, base_url, okr_url, okr_id, objective_id, conn_uuid, cache)
        if prune:
            deleted_ids = replace_target_krs(
                session=session,
                base_url=base_url,
                okr_url=okr_url,
                okr_id=okr_id,
                objective_id=objective_id,
                target_krs=spec.krs,
                conn_uuid=conn_uuid,
                version_cache=cache,
            )
        else:
            deleted_ids = []
            existing_by_text = {kr["text"]: str(kr["id"]) for kr in existing["krs"]}
            target_ids = [existing_by_text[text] for text in spec.krs if text in existing_by_text]
            for text in spec.krs:
                if text not in existing_by_text:
                    target_ids.append(create_kr(session, base_url, okr_url, okr_id, objective_id, text, conn_uuid, cache))
            remaining_ids = [str(kr["id"]) for kr in existing["krs"] if kr["text"] not in spec.krs]
            order_krs(
                session,
                base_url,
                okr_url,
                okr_id,
                objective_id,
                target_ids + remaining_ids,
                conn_uuid,
                cache,
            )
        publish_objective(
            session,
            base_url,
            okr_url,
            okr_id,
            objective_id,
            conn_uuid,
            cache,
            delete_kr_ids=deleted_ids,
        )

    final = summarize(get_detail(session, base_url, okr_url, okr_id))
    final_by_text = {item["objective"]: item for item in final}
    for spec in specs:
        actual = final_by_text.get(spec.objective)
        if actual is None:
            raise RuntimeError(f"Objective was not found after writing: {spec.objective}")
        actual_krs = [kr["text"] for kr in actual["krs"]]
        if actual_krs[: len(spec.krs)] != spec.krs:
            raise RuntimeError(f"KR content did not match after writing: {spec.objective}")
    return public_summary(final)


def write_okr(config: OkrWriteConfig) -> dict[str, Any]:
    specs = parse_specs(config.input_path)
    cookie_path = config.cookies_path.expanduser()
    ensure_cookie_file(cookie_path)
    base_url = config.base_url.rstrip("/") if config.base_url else base_url_from_url(config.url)
    session = build_session(load_cookie_objects(cookie_path), base_url, config.url)
    okr_id, okr_url = resolve_okr_id(session, config.url)
    current = summarize(get_detail(session, base_url, okr_url, okr_id))
    actions = plan_changes(current, specs, config.prune, config.objective_index)
    if not config.apply:
        return {
            "ok": True,
            "dryRun": True,
            "actions": actions,
            "objectiveCount": len(current),
        }

    conn_uuid = random_token()
    if config.objective_index is not None:
        if len(specs) != 1:
            raise ValueError("--objective-index requires exactly one Objective.")
        target = write_target_objective_index(
            session=session,
            base_url=base_url,
            okr_url=okr_url,
            okr_id=okr_id,
            spec=specs[0],
            index=config.objective_index,
            conn_uuid=conn_uuid,
        )
        return {
            "ok": True,
            "dryRun": False,
            "target": target,
        }

    final = write_specs(
        session=session,
        base_url=base_url,
        okr_url=okr_url,
        okr_id=okr_id,
        specs=specs,
        prune=config.prune,
        conn_uuid=conn_uuid,
    )
    return {
        "ok": True,
        "dryRun": False,
        "objectives": final,
    }
