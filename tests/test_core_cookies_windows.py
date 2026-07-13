from __future__ import annotations

import base64
import json
from pathlib import Path
import sqlite3

import pytest
from cryptography.hazmat.primitives.ciphers.aead import AESGCM

from ixf_toolbox.core.cookies.windows_larkshell import (
    decrypt_chromium_cookie_value,
    export_cookies,
    export_cookies_from_db,
    find_cookie_db,
    find_local_state,
    load_local_state_master_key,
    row_to_cookie,
    unwrap_local_state_key,
)


HOST = ".example.test"
HOST_LIKE = "%.example.test%"


def create_cookie_db(tmp_path: Path, rows: list[tuple[str, str, str, bytes, str, int]]) -> Path:
    tmp_path.mkdir(parents=True, exist_ok=True)
    db = tmp_path / "Cookies"
    conn = sqlite3.connect(db)
    conn.execute(
        """
        CREATE TABLE cookies (
            host_key TEXT,
            name TEXT,
            value TEXT,
            encrypted_value BLOB,
            path TEXT,
            is_secure INTEGER
        )
        """
    )
    conn.executemany("INSERT INTO cookies VALUES (?, ?, ?, ?, ?, ?)", rows)
    conn.commit()
    conn.close()
    return db


def test_find_cookie_db_returns_explicit_path(tmp_path: Path) -> None:
    db = tmp_path / "Cookies"
    db.write_bytes(b"sqlite")

    assert find_cookie_db(cookies_db=db, app_data=None) == db


def test_find_cookie_db_raises_when_missing(tmp_path: Path) -> None:
    with pytest.raises(FileNotFoundError, match="Windows LarkShell cookie DB not found"):
        find_cookie_db(cookies_db=None, app_data=tmp_path)


def test_find_local_state_returns_explicit_path(tmp_path: Path) -> None:
    local_state = tmp_path / "Local State"
    local_state.write_text("{}", encoding="utf-8")

    assert find_local_state(local_state=local_state, app_data=None, cookies_db=None) == local_state


def test_find_local_state_discovers_user_data_from_cookie_db(tmp_path: Path) -> None:
    user_data = tmp_path / "LarkShell" / "User Data"
    db = user_data / "Default" / "Network" / "Cookies"
    db.parent.mkdir(parents=True)
    db.write_bytes(b"sqlite")
    local_state = user_data / "Local State"
    local_state.write_text("{}", encoding="utf-8")

    assert find_local_state(local_state=None, app_data=None, cookies_db=db) == local_state


def test_row_to_cookie_uses_plain_value_when_present() -> None:
    row = {
        "host_key": HOST,
        "name": "_csrf_token",
        "value": "plain",
        "encrypted_value": b"",
        "path": "/",
        "is_secure": 1,
    }

    cookie = row_to_cookie(row, decrypt_value=lambda value: "decrypted")

    assert cookie["value"] == "plain"
    assert cookie["secure"] is True


def test_row_to_cookie_decrypts_encrypted_value() -> None:
    row = {
        "host_key": HOST,
        "name": "session",
        "value": "",
        "encrypted_value": b"encrypted",
        "path": "/",
        "is_secure": 0,
    }

    cookie = row_to_cookie(row, decrypt_value=lambda value: "secret")

    assert cookie["value"] == "secret"
    assert cookie["secure"] is False


def test_unwrap_local_state_key_strips_dpapi_prefix() -> None:
    wrapped = b"wrapped-local-state-key"
    unwrapped = b"k" * 32
    encrypted_key = base64.b64encode(b"DPAPI" + wrapped).decode("ascii")

    key = unwrap_local_state_key(encrypted_key, dpapi_unprotect=lambda value: unwrapped)

    assert key == unwrapped


def test_decrypt_chromium_cookie_value_decrypts_versioned_aes_gcm_blob() -> None:
    master_key = b"k" * 32
    nonce = b"n" * 12
    encrypted = AESGCM(master_key).encrypt(nonce, b"synthetic-csrf", None)

    value = decrypt_chromium_cookie_value(b"v10" + nonce + encrypted, master_key=master_key)

    assert value == "synthetic-csrf"


def test_decrypt_chromium_cookie_value_falls_back_to_legacy_dpapi() -> None:
    value = decrypt_chromium_cookie_value(
        b"legacy-encrypted",
        master_key=b"k" * 32,
        dpapi_unprotect=lambda encrypted: b"legacy-cookie",
    )

    assert value == "legacy-cookie"


def test_export_cookies_from_db_writes_browser_cookie_json(tmp_path: Path) -> None:
    db = create_cookie_db(tmp_path, [(HOST, "_csrf_token", "", b"encrypted", "/", 1)])
    output = tmp_path / "cookies.json"

    payload = export_cookies_from_db(
        db,
        output,
        HOST_LIKE,
        decrypt_value=lambda value: "csrf",
    )

    assert payload["cookieCount"] == 1
    assert payload["hasCsrf"] is True
    cookies = json.loads(output.read_text(encoding="utf-8"))
    assert [cookie["value"] for cookie in cookies] == ["csrf"]


def test_export_cookies_from_db_raises_when_csrf_value_is_empty(tmp_path: Path) -> None:
    db = create_cookie_db(tmp_path, [(HOST, "_csrf_token", "", b"", "/", 1)])

    with pytest.raises(RuntimeError, match="do not contain a non-empty _csrf_token"):
        export_cookies_from_db(
            db,
            tmp_path / "cookies.json",
            HOST_LIKE,
            decrypt_value=lambda value: "unused",
        )


def test_export_cookies_from_db_raises_when_csrf_cookie_is_missing(tmp_path: Path) -> None:
    db = create_cookie_db(tmp_path, [(HOST, "session", "plain", b"", "/", 1)])

    with pytest.raises(RuntimeError, match="do not contain a non-empty _csrf_token"):
        export_cookies_from_db(
            db,
            tmp_path / "cookies.json",
            HOST_LIKE,
            decrypt_value=lambda value: "unused",
        )


def test_load_local_state_master_key_normalizes_bad_json(tmp_path: Path) -> None:
    local_state = tmp_path / "Local State"
    local_state.write_text("{not-json", encoding="utf-8")

    with pytest.raises(RuntimeError, match="Windows LarkShell Local State is invalid"):
        load_local_state_master_key(local_state, dpapi_unprotect=lambda value: b"unused")


def test_load_local_state_master_key_normalizes_empty_json(tmp_path: Path) -> None:
    local_state = tmp_path / "Local State"
    local_state.write_text("", encoding="utf-8")

    with pytest.raises(RuntimeError, match="Windows LarkShell Local State is invalid"):
        load_local_state_master_key(local_state, dpapi_unprotect=lambda value: b"unused")


def test_load_local_state_master_key_normalizes_non_object_json(tmp_path: Path) -> None:
    local_state = tmp_path / "Local State"
    local_state.write_text("[]", encoding="utf-8")

    with pytest.raises(RuntimeError, match="Windows LarkShell Local State is invalid"):
        load_local_state_master_key(local_state, dpapi_unprotect=lambda value: b"unused")


def test_load_local_state_master_key_normalizes_bad_base64(tmp_path: Path) -> None:
    local_state = tmp_path / "Local State"
    local_state.write_text(
        json.dumps({"os_crypt": {"encrypted_key": "not valid base64!"}}),
        encoding="utf-8",
    )

    with pytest.raises(RuntimeError, match="Windows LarkShell Local State key is invalid"):
        load_local_state_master_key(local_state, dpapi_unprotect=lambda value: b"unused")


def test_load_local_state_master_key_normalizes_malformed_base64(tmp_path: Path) -> None:
    local_state = tmp_path / "Local State"
    local_state.write_text(
        json.dumps({"os_crypt": {"encrypted_key": "abc"}}),
        encoding="utf-8",
    )

    with pytest.raises(RuntimeError, match="Windows LarkShell Local State key is invalid"):
        load_local_state_master_key(local_state, dpapi_unprotect=lambda value: b"unused")


def test_decrypt_chromium_cookie_value_normalizes_invalid_aes_gcm_tag() -> None:
    master_key = b"k" * 32
    nonce = b"n" * 12
    encrypted = AESGCM(master_key).encrypt(nonce, b"csrf", None)

    with pytest.raises(RuntimeError, match="Could not decrypt Windows Chromium cookie value"):
        decrypt_chromium_cookie_value(
            b"v10" + nonce + encrypted[:-1] + b"x",
            master_key=master_key,
        )


def test_decrypt_chromium_cookie_value_normalizes_utf8_decode_errors() -> None:
    with pytest.raises(RuntimeError, match="not valid UTF-8"):
        decrypt_chromium_cookie_value(
            b"legacy-encrypted",
            master_key=b"k" * 32,
            dpapi_unprotect=lambda encrypted: b"\xff",
        )


def test_decrypt_chromium_cookie_value_normalizes_dpapi_failures() -> None:
    def fail_dpapi(encrypted: bytes) -> bytes:
        raise OSError("fixture-dpapi-failure")

    with pytest.raises(RuntimeError, match="Could not decrypt Windows cookie value"):
        decrypt_chromium_cookie_value(
            b"legacy-encrypted",
            master_key=b"k" * 32,
            dpapi_unprotect=fail_dpapi,
        )


def test_unwrap_local_state_key_normalizes_dpapi_failures() -> None:
    def fail_dpapi(encrypted: bytes) -> bytes:
        raise OSError("fixture-dpapi-failure")

    encrypted_key = base64.b64encode(b"DPAPIwrapped").decode("ascii")

    with pytest.raises(RuntimeError, match="Could not decrypt Windows LarkShell Local State key"):
        unwrap_local_state_key(encrypted_key, dpapi_unprotect=fail_dpapi)


def test_export_cookies_decrypts_versioned_aes_gcm_cookie_with_local_state(
    tmp_path: Path,
) -> None:
    master_key = b"k" * 32
    wrapped_key = b"wrapped-local-state-key"
    user_data = tmp_path / "LarkShell" / "User Data"
    local_state = user_data / "Local State"
    local_state.parent.mkdir(parents=True)
    local_state.write_text(
        json.dumps(
            {
                "os_crypt": {
                    "encrypted_key": base64.b64encode(b"DPAPI" + wrapped_key).decode("ascii")
                }
            }
        ),
        encoding="utf-8",
    )
    nonce = b"n" * 12
    encrypted_value = b"v11" + nonce + AESGCM(master_key).encrypt(nonce, b"csrf", None)
    db = create_cookie_db(
        user_data / "Default" / "Network",
        [(HOST, "_csrf_token", "", encrypted_value, "/", 1)],
    )

    def fake_dpapi_unprotect(value: bytes) -> bytes:
        assert value == wrapped_key
        return master_key

    payload = export_cookies(
        output=tmp_path / "cookies.json",
        cookies_db=db,
        local_state=local_state,
        host_like=HOST_LIKE,
        dpapi_unprotect=fake_dpapi_unprotect,
    )

    assert payload["ok"] is True
    assert payload["cookieCount"] == 1
    assert payload["hasCsrf"] is True
    cookies = json.loads((tmp_path / "cookies.json").read_text(encoding="utf-8"))
    assert [cookie["value"] for cookie in cookies] == ["csrf"]


def test_export_cookies_uses_injected_dpapi_decryptor(tmp_path: Path) -> None:
    db = create_cookie_db(tmp_path, [(HOST, "_csrf_token", "", b"encrypted", "/", 1)])

    def fake_dpapi_unprotect(value: bytes) -> bytes:
        assert value == b"encrypted"
        return b"csrf"

    payload = export_cookies(
        output=tmp_path / "cookies.json",
        cookies_db=db,
        host_like=HOST_LIKE,
        dpapi_unprotect=fake_dpapi_unprotect,
    )

    assert payload["ok"] is True
    assert payload["cookieCount"] == 1


def test_export_cookies_accepts_output_as_positional_argument(tmp_path: Path) -> None:
    db = create_cookie_db(tmp_path, [(HOST, "_csrf_token", "plain", b"", "/", 1)])

    payload = export_cookies(tmp_path / "cookies.json", cookies_db=db, host_like=HOST_LIKE)

    assert payload["ok"] is True
    assert payload["cookieCount"] == 1


def test_export_cookies_uses_appdata_from_environment(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    db_parent = tmp_path / "LarkShell" / "User Data" / "Default" / "Network"
    create_cookie_db(db_parent, [(HOST, "_csrf_token", "plain", b"", "/", 1)])
    monkeypatch.setenv("APPDATA", str(tmp_path))

    payload = export_cookies(tmp_path / "cookies.json", host_like=HOST_LIKE)

    assert payload["ok"] is True
    assert payload["cookieCount"] == 1
