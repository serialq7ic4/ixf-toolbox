from __future__ import annotations

import http.cookiejar
import json
from pathlib import Path
from typing import Any


DEFAULT_COOKIES = "/tmp/ixunfei_profile_explorer_cookies.json"


def load_cookie_objects(cookie_path: Path) -> list[dict[str, Any]]:
    path = cookie_path.expanduser()
    if not path.exists():
        raise FileNotFoundError(f"Cookie file not found: {path}")
    if path.suffix == ".json":
        data = json.loads(path.read_text(encoding="utf-8"))
        if not isinstance(data, list):
            raise ValueError("Cookie JSON must be a list of browser cookie objects.")
        return [item for item in data if isinstance(item, dict)]

    jar = http.cookiejar.MozillaCookieJar(str(path))
    jar.load(ignore_discard=True, ignore_expires=True)
    return [
        {
            "name": cookie.name,
            "value": cookie.value,
            "domain": cookie.domain,
            "path": cookie.path,
            "secure": cookie.secure,
        }
        for cookie in jar
    ]


def write_cookie_json(output: Path, cookies: list[dict[str, Any]]) -> Path:
    path = output.expanduser()
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(cookies, ensure_ascii=False, indent=2), encoding="utf-8")
    try:
        path.chmod(0o600)
    except OSError:
        pass
    return path


def cookie_diagnostics(cookie_path: Path) -> dict[str, object]:
    path = cookie_path.expanduser()
    if not path.exists():
        return {
            "ok": False,
            "exists": False,
            "path": str(path),
            "cookieCount": 0,
            "cookieNames": [],
            "hasCsrf": False,
            "hasLgwCsrf": False,
        }
    try:
        cookies = load_cookie_objects(path)
    except Exception as exc:
        return {
            "ok": False,
            "exists": True,
            "path": str(path),
            "cookieCount": 0,
            "cookieNames": [],
            "hasCsrf": False,
            "hasLgwCsrf": False,
            "error": f"{type(exc).__name__}: {exc}",
        }
    names = sorted({str(cookie.get("name")) for cookie in cookies if cookie.get("name")})
    return {
        "ok": True,
        "exists": True,
        "path": str(path),
        "cookieCount": len(cookies),
        "cookieNames": names,
        "hasCsrf": any(
            cookie.get("name") == "_csrf_token" and bool(cookie.get("value"))
            for cookie in cookies
        ),
        "hasLgwCsrf": any(
            cookie.get("name") == "lgw_csrf_token" and bool(cookie.get("value"))
            for cookie in cookies
        ),
    }
