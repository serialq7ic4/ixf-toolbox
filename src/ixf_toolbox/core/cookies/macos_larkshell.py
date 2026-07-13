from __future__ import annotations

import hashlib
from pathlib import Path
import sqlite3
import subprocess
from typing import Any

from ixf_toolbox.core.cookies.common import write_cookie_json


DEFAULT_APP_SUPPORT = "~/Library/Application Support/LarkShell-ka-kaahyz17"
DEFAULT_KEYCHAIN_SERVICE = "Suite App Safe Storage"
DEFAULT_HOST_LIKE = "%xfchat.iflytek.com%"
CHROMIUM_EPOCH_DELTA_SECONDS = 11644473600


def newest_cookie_db(app_support: Path) -> Path:
    candidates = [
        path
        for path in app_support.expanduser().glob("aha/users/*/profile_explorer/Cookies")
        if path.is_file()
    ]
    if not candidates:
        raise FileNotFoundError(f"No profile_explorer/Cookies found under {app_support}")
    return max(candidates, key=lambda path: path.stat().st_mtime)


def find_keychain_password(service: str, account: str = "") -> str:
    command = ["security", "find-generic-password", "-w", "-s", service]
    if account:
        command.extend(["-a", account])
    password = subprocess.check_output(command, text=True).strip("\n")
    if not password:
        raise RuntimeError(f"Empty Keychain password for service={service!r}")
    return password


def key_from_keychain(service: str, account: str = "") -> bytes:
    password = find_keychain_password(service, account)
    return hashlib.pbkdf2_hmac("sha1", password.encode(), b"saltysalt", 1003, dklen=16)


def decrypt_cookie_value(host: str, encrypted_value: bytes, key: bytes) -> str:
    try:
        from cryptography.hazmat.backends import default_backend
        from cryptography.hazmat.primitives.ciphers import Cipher, algorithms, modes
        from cryptography.hazmat.primitives.padding import PKCS7
    except ImportError as exc:
        raise RuntimeError(
            "Missing dependency: cryptography. Install with `pip install 'ixf-toolbox[crypto]'`."
        ) from exc

    if not encrypted_value:
        return ""
    ciphertext = encrypted_value[3:] if encrypted_value.startswith((b"v10", b"v11")) else encrypted_value
    cipher = Cipher(algorithms.AES(key), modes.CBC(b" " * 16), backend=default_backend())
    decryptor = cipher.decryptor()
    padded = decryptor.update(ciphertext) + decryptor.finalize()
    unpadder = PKCS7(128).unpadder()
    plain = unpadder.update(padded) + unpadder.finalize()

    host_digest = hashlib.sha256(host.encode()).digest()
    if plain.startswith(host_digest):
        plain = plain[32:]
    return plain.decode("utf-8")


def chromium_expires_to_unix(expires_utc: int) -> int:
    return max(0, int(expires_utc / 1_000_000 - CHROMIUM_EPOCH_DELTA_SECONDS))


def same_site_name(value: int) -> str | None:
    if value == -1:
        return "None"
    if value == 0:
        return "Lax"
    if value == 1:
        return "Strict"
    return None


def read_cookie_rows(cookies_db: Path, host_like: str) -> list[dict[str, Any]]:
    con = sqlite3.connect(f"file:{cookies_db}?mode=ro", uri=True)
    try:
        rows = con.execute(
            """
            select host_key, name, value, encrypted_value, path, expires_utc,
                   is_secure, is_httponly, samesite
            from cookies
            where host_key like ?
            order by host_key, name
            """,
            (host_like,),
        ).fetchall()
    finally:
        con.close()

    return [
        {
            "host": host,
            "name": name,
            "value": value,
            "encrypted_value": encrypted_value or b"",
            "path": path,
            "expires_utc": expires_utc,
            "is_secure": is_secure,
            "is_httponly": is_httponly,
            "samesite": samesite,
        }
        for host, name, value, encrypted_value, path, expires_utc, is_secure, is_httponly, samesite in rows
    ]


def export_cookies(
    *,
    output: Path,
    app_support: Path = Path(DEFAULT_APP_SUPPORT),
    cookies_db: Path | None = None,
    host_like: str = DEFAULT_HOST_LIKE,
    keychain_service: str = DEFAULT_KEYCHAIN_SERVICE,
    keychain_account: str = "",
) -> dict[str, object]:
    db_path = cookies_db.expanduser() if cookies_db else newest_cookie_db(app_support)
    rows = read_cookie_rows(db_path, host_like)
    needs_decrypt = any(not row["value"] and row["encrypted_value"] for row in rows)
    key = key_from_keychain(keychain_service, keychain_account) if needs_decrypt else None

    cookies: list[dict[str, Any]] = []
    for row in rows:
        cookie_value = row["value"] or decrypt_cookie_value(
            row["host"],
            row["encrypted_value"],
            key or b"",
        )
        item: dict[str, Any] = {
            "name": row["name"],
            "value": cookie_value,
            "domain": row["host"],
            "path": row["path"] or "/",
            "secure": bool(row["is_secure"]),
            "httpOnly": bool(row["is_httponly"]),
        }
        if row["expires_utc"]:
            item["expires"] = chromium_expires_to_unix(int(row["expires_utc"]))
        same_site = same_site_name(int(row["samesite"]))
        if same_site:
            item["sameSite"] = same_site
        cookies.append(item)

    has_csrf = any(cookie["name"] == "_csrf_token" and cookie["value"] for cookie in cookies)
    if not has_csrf:
        raise RuntimeError("Exported cookies do not contain a non-empty _csrf_token.")
    out_path = write_cookie_json(output, cookies)
    return {
        "ok": True,
        "provider": "macos-larkshell",
        "cookieCount": len(cookies),
        "hasCsrf": has_csrf,
        "output": str(out_path),
    }
