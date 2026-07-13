from __future__ import annotations

import argparse
import json
import os
import sys
from pathlib import Path

from ixf_toolbox import __version__
from ixf_toolbox.core.cookies import (
    DEFAULT_APP_SUPPORT,
    DEFAULT_COOKIES,
    DEFAULT_HOST_LIKE,
    DEFAULT_KEYCHAIN_SERVICE,
    export_cookie_session,
)
from ixf_toolbox.delegate import run_command
from ixf_toolbox.doctor import collect_diagnostics, format_diagnostics, to_json
from ixf_toolbox.setup import install_skill_wrappers, packaged_project_root
from ixf_toolbox.update import DEFAULT_RELEASE_REPO, check_latest_release


DOCS_READ_COMMANDS = {"read", "outline", "chunk", "cleanup", "inspect"}


def print_usage() -> None:
    print(
        "usage: ixf [--version] "
        "{docs,okr,cookies,doctor,setup,update} ...",
        file=sys.stderr,
    )


def run_docs(args: list[str]) -> int:
    if not args:
        print("ERROR docs requires a subcommand.", file=sys.stderr)
        return 2
    command, rest = args[0], args[1:]
    if command in DOCS_READ_COMMANDS:
        return run_command("ixfdoc", [command, *rest])
    if command == "publish":
        return run_command("ixfwrite", ["docx", "publish", *rest])
    print(f"ERROR unsupported docs subcommand: {command}", file=sys.stderr)
    return 2


def run_okr(args: list[str]) -> int:
    if not args:
        print("ERROR okr requires a subcommand.", file=sys.stderr)
        return 2
    command, rest = args[0], args[1:]
    if command == "read":
        return run_command("ixfdoc", ["read", *rest])
    if command == "write":
        return run_command("ixfwrite", ["okr", "write", *rest])
    print(f"ERROR unsupported okr subcommand: {command}", file=sys.stderr)
    return 2


def run_setup(args: list[str]) -> int:
    parser = argparse.ArgumentParser(prog="ixf setup")
    subparsers = parser.add_subparsers(dest="setup_command")
    skills = subparsers.add_parser("skills")
    skills.add_argument("--runtimes", default="auto")
    skills.add_argument("--force", action="store_true")
    skills.add_argument("--json", action="store_true", dest="as_json")
    parsed = parser.parse_args(args)
    if parsed.setup_command != "skills":
        parser.print_help(sys.stderr)
        return 2
    try:
        payload = install_skill_wrappers(
            packaged_project_root(),
            Path.home(),
            parsed.runtimes.split(","),
            bool(parsed.force),
            dict(os.environ),
        )
    except ValueError as exc:
        print(f"ERROR {exc}", file=sys.stderr)
        return 2
    if parsed.as_json:
        print(json.dumps(payload, ensure_ascii=False))
    else:
        print(f"installed {len(payload['installed'])} wrapper(s)")
    return 0


def run_update(args: list[str]) -> int:
    parser = argparse.ArgumentParser(prog="ixf update")
    subparsers = parser.add_subparsers(dest="update_command")
    check = subparsers.add_parser("check")
    check.add_argument("--repo", default=DEFAULT_RELEASE_REPO)
    check.add_argument("--json", action="store_true", dest="as_json")
    skills = subparsers.add_parser("skills")
    skills.add_argument("--runtimes", default="auto")
    skills.add_argument("--json", action="store_true", dest="as_json")
    parsed = parser.parse_args(args)
    if parsed.update_command == "check":
        try:
            payload = check_latest_release(repo=parsed.repo, current_version=__version__)
        except Exception as exc:
            print(f"ERROR update check failed: {exc}", file=sys.stderr)
            return 10
        if parsed.as_json:
            print(json.dumps(payload, ensure_ascii=False))
        else:
            print(f"current {payload['currentVersion']}")
            print(f"latest {payload['latestVersion']}")
            print(f"updateAvailable {str(payload['updateAvailable']).lower()}")
            if payload["installCommand"]:
                print(payload["installCommand"])
        return 0
    if parsed.update_command == "skills":
        payload = install_skill_wrappers(
            packaged_project_root(),
            Path.home(),
            parsed.runtimes.split(","),
            True,
            dict(os.environ),
        )
        if parsed.as_json:
            print(json.dumps(payload, ensure_ascii=False))
        else:
            print(f"installed {len(payload['installed'])} wrapper(s)")
        return 0
    parser.print_help(sys.stderr)
    return 2


def run_doctor(args: list[str]) -> int:
    parser = argparse.ArgumentParser(prog="ixf doctor")
    parser.add_argument("--cookies", default=DEFAULT_COOKIES)
    parser.add_argument("--json", action="store_true", dest="as_json")
    parsed = parser.parse_args(args)
    payload = collect_diagnostics(
        home=Path.home(),
        env=dict(os.environ),
        cookies_path=Path(parsed.cookies),
    )
    if parsed.as_json:
        print(to_json(payload))
    else:
        print(format_diagnostics(payload), end="")
    return 0 if payload["ok"] else 1


def run_cookies(args: list[str]) -> int:
    parser = argparse.ArgumentParser(prog="ixf cookies")
    subparsers = parser.add_subparsers(dest="cookies_command")
    export = subparsers.add_parser("export", help="Export local LarkShell cookies.")
    export.add_argument(
        "--provider",
        default="auto",
        choices=["auto", "macos-larkshell", "windows-larkshell"],
    )
    export.add_argument("--output", default=DEFAULT_COOKIES)
    export.add_argument("--app-support", default=DEFAULT_APP_SUPPORT)
    export.add_argument("--cookies-db", default="")
    export.add_argument("--host-like", default=DEFAULT_HOST_LIKE)
    export.add_argument("--keychain-service", default=DEFAULT_KEYCHAIN_SERVICE)
    export.add_argument("--keychain-account", default="")
    export.add_argument("--app-data", default="")
    export.add_argument("--local-state", default="")
    parsed = parser.parse_args(args)
    if parsed.cookies_command != "export":
        parser.print_help(sys.stderr)
        return 2
    try:
        payload = export_cookie_session(
            provider=parsed.provider,
            output=Path(parsed.output),
            app_support=Path(parsed.app_support),
            cookies_db=Path(parsed.cookies_db) if parsed.cookies_db else None,
            host_like=parsed.host_like,
            keychain_service=parsed.keychain_service,
            keychain_account=parsed.keychain_account,
            app_data=Path(parsed.app_data) if parsed.app_data else None,
            local_state=Path(parsed.local_state) if parsed.local_state else None,
        )
    except Exception as exc:
        error = {
            "ok": False,
            "error": {
                "type": "cookie",
                "subtype": "cookie_export_failed",
                "message": str(exc),
                "hint": "Confirm the desktop client is logged in and retry `ixf cookies export`.",
                "retryable": False,
            },
        }
        print(f"ERROR {exc}", file=sys.stderr)
        print(json.dumps(error, ensure_ascii=False), file=sys.stderr)
        return 6
    print(json.dumps(payload, ensure_ascii=False))
    return 0


def main(argv: list[str] | None = None) -> int:
    args = list(sys.argv[1:] if argv is None else argv)
    if args == ["--version"]:
        print(f"ixf {__version__}")
        return 0
    if not args or args[0] in {"-h", "--help"}:
        print_usage()
        return 0 if args else 2

    command, rest = args[0], args[1:]
    if command == "docs":
        return run_docs(rest)
    if command == "okr":
        return run_okr(rest)
    if command == "cookies":
        return run_cookies(rest)
    if command == "doctor":
        return run_doctor(rest)
    if command == "setup":
        return run_setup(rest)
    if command == "update":
        return run_update(rest)
    print(f"ERROR unsupported command: {command}", file=sys.stderr)
    print_usage()
    return 2
