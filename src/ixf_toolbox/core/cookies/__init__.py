from __future__ import annotations

import platform
from pathlib import Path

from ixf_toolbox.core.cookies.common import (
    DEFAULT_COOKIES,
    cookie_diagnostics,
    load_cookie_objects,
    write_cookie_json,
)
from ixf_toolbox.core.cookies.macos_larkshell import (
    DEFAULT_APP_SUPPORT,
    DEFAULT_HOST_LIKE,
    DEFAULT_KEYCHAIN_SERVICE,
)


def export_cookie_session(
    *,
    provider: str,
    output: Path,
    app_support: Path = Path(DEFAULT_APP_SUPPORT),
    cookies_db: Path | None = None,
    host_like: str = DEFAULT_HOST_LIKE,
    keychain_service: str = DEFAULT_KEYCHAIN_SERVICE,
    keychain_account: str = "",
    app_data: Path | None = None,
    local_state: Path | None = None,
) -> dict[str, object]:
    selected = provider
    if selected == "auto":
        system = platform.system().lower()
        if system == "darwin":
            selected = "macos-larkshell"
        elif system == "windows":
            selected = "windows-larkshell"
        else:
            raise RuntimeError("Automatic cookie export supports macOS and Windows.")

    if selected == "macos-larkshell":
        from ixf_toolbox.core.cookies.macos_larkshell import export_cookies

        return export_cookies(
            output=output,
            app_support=app_support,
            cookies_db=cookies_db,
            host_like=host_like,
            keychain_service=keychain_service,
            keychain_account=keychain_account,
        )
    if selected == "windows-larkshell":
        from ixf_toolbox.core.cookies.windows_larkshell import export_cookies

        return export_cookies(
            output=output,
            app_data=app_data,
            cookies_db=cookies_db,
            local_state=local_state,
            host_like=host_like,
        )
    raise ValueError(f"Unsupported cookie provider: {provider}")


__all__ = [
    "DEFAULT_COOKIES",
    "DEFAULT_APP_SUPPORT",
    "DEFAULT_HOST_LIKE",
    "DEFAULT_KEYCHAIN_SERVICE",
    "cookie_diagnostics",
    "export_cookie_session",
    "load_cookie_objects",
    "write_cookie_json",
]
