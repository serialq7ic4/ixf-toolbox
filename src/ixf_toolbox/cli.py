from __future__ import annotations

import argparse
import json
import os
import sys
from pathlib import Path

import requests

from ixf_toolbox import __version__
from ixf_toolbox.core.cookies import (
    DEFAULT_APP_SUPPORT,
    DEFAULT_COOKIES,
    DEFAULT_HOST_LIKE,
    DEFAULT_KEYCHAIN_SERVICE,
    export_cookie_session,
)
from ixf_toolbox.core.docs import (
    DEFAULT_SPACE_API,
    DocxPublishConfig,
    build_outline,
    cleanup_outputs,
    inspect_source,
    publish_markdown,
    read_sources,
    render_chunk,
    write_outputs,
)
from ixf_toolbox.core.okr import OkrWriteConfig, read_okr_url, write_okr
from ixf_toolbox.doctor import collect_diagnostics, format_diagnostics, to_json
from ixf_toolbox.setup import install_skill_wrappers, packaged_project_root
from ixf_toolbox.update import DEFAULT_RELEASE_REPO, check_latest_release


EXIT_CODES = {
    "bad_args": 2,
    "cookie_file_missing": 5,
    "cookie_export_failed": 6,
    "cookie_file_invalid": 7,
    "cookie_csrf_missing": 8,
    "remote_read_failed": 9,
    "update_check_failed": 10,
}


def print_usage() -> None:
    print(
        "usage: ixf [--version] "
        "{docs,okr,cookies,doctor,setup,update} ...",
        file=sys.stderr,
    )


def structured_error(
    *,
    error_type: str,
    subtype: str,
    message: str,
    hint: str,
    retryable: bool = False,
) -> int:
    print(f"ERROR {message}", file=sys.stderr)
    if hint:
        print(f"HINT {hint}", file=sys.stderr)
    print(
        json.dumps(
            {
                "ok": False,
                "error": {
                    "type": error_type,
                    "subtype": subtype,
                    "message": message,
                    "hint": hint,
                    "retryable": retryable,
                },
            },
            ensure_ascii=False,
        ),
        file=sys.stderr,
    )
    return EXIT_CODES.get(subtype, 1)


def run_docs_read(args: list[str]) -> int:
    parser = argparse.ArgumentParser(prog="ixf docs read")
    parser.add_argument("sources", nargs="*")
    parser.add_argument("--out-dir", default="")
    parser.add_argument("--expand-sheets", action="store_true")
    parser.add_argument("--download-images", action="store_true")
    parser.add_argument("--print-manifest", action="store_true")
    parser.add_argument("--cleanup", action="store_true")
    parser.add_argument("--cookies", default=DEFAULT_COOKIES)
    parser.add_argument("--space-api", default=DEFAULT_SPACE_API)
    parsed = parser.parse_args(args)
    if not parsed.sources:
        return structured_error(
            error_type="usage",
            subtype="bad_args",
            message="read requires at least one source.",
            hint="Run `ixf docs read <url-or-file>... --out-dir <dir>`.",
        )
    if parsed.download_images and not parsed.out_dir:
        return structured_error(
            error_type="usage",
            subtype="bad_args",
            message="--download-images requires --out-dir.",
            hint="Pass `--out-dir <dir>` so image assets can be written locally.",
        )
    out_dir = Path(parsed.out_dir).expanduser() if parsed.out_dir else None
    try:
        results = read_sources(
            parsed.sources,
            cookies_path=Path(parsed.cookies).expanduser(),
            space_api=parsed.space_api,
            expand_sheets=parsed.expand_sheets,
            download_images=parsed.download_images,
            output_root=out_dir,
        )
    except FileNotFoundError as exc:
        message = str(exc)
        if message.startswith("Cookie file not found:"):
            return structured_error(
                error_type="cookie",
                subtype="cookie_file_missing",
                message=message,
                hint="Run `ixf cookies export --provider auto --output <path>` or pass --cookies.",
            )
        return structured_error(
            error_type="usage",
            subtype="bad_args",
            message=message,
            hint="Pass an existing local file path or a supported i讯飞 document URL.",
        )
    except ValueError as exc:
        message = str(exc)
        if message == "Cookie jar does not contain _csrf_token.":
            return structured_error(
                error_type="cookie",
                subtype="cookie_csrf_missing",
                message=message,
                hint="Run `ixf cookies export --provider auto --output <path>` to refresh the local desktop session cookies.",
            )
        return structured_error(
            error_type="cookie",
            subtype="cookie_file_invalid",
            message=message,
            hint="Run `ixf cookies export --provider auto --output <path>` or pass a valid --cookies file.",
        )
    except (requests.RequestException, RuntimeError) as exc:
        return structured_error(
            error_type="remote",
            subtype="remote_read_failed",
            message=str(exc),
            hint="Check network access, document permissions, and whether the local desktop session is still valid.",
            retryable=True,
        )
    if out_dir is not None:
        manifest = write_outputs(results, out_dir)
        if parsed.print_manifest:
            print(json.dumps(manifest, ensure_ascii=False, indent=2))
        if parsed.cleanup:
            cleanup_outputs(manifest, out_dir)
        return 0

    multiple = len(results) > 1
    for result in results:
        if multiple:
            print(f"=== {result['source']} ({result['kind']}) ===")
        sys.stdout.write(str(result["content"]))
        if not str(result["content"]).endswith("\n"):
            print()
    return 0


def run_docs_cleanup(args: list[str]) -> int:
    parser = argparse.ArgumentParser(prog="ixf docs cleanup")
    parser.add_argument("out_dir")
    parsed = parser.parse_args(args)
    out_dir = Path(parsed.out_dir).expanduser()
    manifest_path = out_dir / "manifest.json"
    if not manifest_path.exists():
        return structured_error(
            error_type="usage",
            subtype="bad_args",
            message=f"manifest not found: {manifest_path}",
            hint="Pass the output directory created by `ixf docs read --out-dir`.",
        )
    try:
        manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError):
        return structured_error(
            error_type="usage",
            subtype="bad_args",
            message=f"invalid manifest: {manifest_path}",
            hint="Pass an intact output directory created by `ixf docs read --out-dir`.",
        )
    if not isinstance(manifest, dict) or not all(
        isinstance(key, str) and isinstance(value, dict)
        for key, value in manifest.items()
    ):
        return structured_error(
            error_type="usage",
            subtype="bad_args",
            message=f"invalid manifest: {manifest_path}",
            hint="Pass an intact output directory created by `ixf docs read --out-dir`.",
        )
    cleanup_outputs(manifest, out_dir)
    return 0


def read_chunk_source(source: str, target_chars: int) -> tuple[Path, str]:
    path = Path(source).expanduser()
    if not path.exists() or not path.is_file():
        raise FileNotFoundError(f"local file not found: {path}")
    if target_chars <= 0:
        raise ValueError("target_chars must be positive.")
    return path, path.read_text(encoding="utf-8")


def run_docs_outline(args: list[str]) -> int:
    parser = argparse.ArgumentParser(prog="ixf docs outline")
    parser.add_argument("source")
    parser.add_argument("--target-chars", type=int, default=12000)
    parser.add_argument("--json", action="store_true", dest="as_json")
    parsed = parser.parse_args(args)
    try:
        path, markdown = read_chunk_source(parsed.source, parsed.target_chars)
        outline = build_outline(markdown, parsed.target_chars)
    except (FileNotFoundError, ValueError) as exc:
        return structured_error(
            error_type="usage",
            subtype="bad_args",
            message=str(exc),
            hint="Pass a generated Markdown file and positive --target-chars.",
        )
    payload = {
        "ok": True,
        "file": str(path),
        "selectedHeadingLevel": outline.selected_heading_level,
        "chunks": [
            {
                "index": chunk.index,
                "breadcrumb": chunk.breadcrumb,
                "startLine": chunk.start_line,
                "endLine": chunk.end_line,
                "charCount": chunk.char_count,
                "imagePaths": list(chunk.image_paths),
            }
            for chunk in outline.chunks
        ],
    }
    if parsed.as_json:
        print(json.dumps(payload, ensure_ascii=False))
    else:
        for chunk in payload["chunks"]:
            print(
                f"{chunk['index']}\t{chunk['startLine']}-{chunk['endLine']}\t"
                f"{chunk['breadcrumb']}"
            )
    return 0


def run_docs_chunk(args: list[str]) -> int:
    parser = argparse.ArgumentParser(prog="ixf docs chunk")
    parser.add_argument("source")
    parser.add_argument("--index", type=int, required=True)
    parser.add_argument("--target-chars", type=int, default=12000)
    parsed = parser.parse_args(args)
    try:
        _, markdown = read_chunk_source(parsed.source, parsed.target_chars)
        outline = build_outline(markdown, parsed.target_chars)
    except (FileNotFoundError, ValueError) as exc:
        return structured_error(
            error_type="usage",
            subtype="bad_args",
            message=str(exc),
            hint="Pass a generated Markdown file and positive --target-chars.",
        )
    if parsed.index < 1 or parsed.index > len(outline.chunks):
        return structured_error(
            error_type="usage",
            subtype="bad_args",
            message=f"chunk index out of range: {parsed.index}",
            hint=f"Pass an index from 1 to {len(outline.chunks)}.",
        )
    chunk = outline.chunks[parsed.index - 1]
    breadcrumb = chunk.breadcrumb.replace("\\", "\\\\").replace('"', '\\"')
    print(f'[chunk {chunk.index}/{len(outline.chunks)} breadcrumb="{breadcrumb}"]')
    print()
    sys.stdout.write(render_chunk(markdown, outline, parsed.index))
    return 0


def run_docs_inspect(args: list[str]) -> int:
    parser = argparse.ArgumentParser(prog="ixf docs inspect")
    parser.add_argument("source")
    parser.add_argument("--json", action="store_true", dest="as_json")
    parsed = parser.parse_args(args)
    try:
        payload = inspect_source(parsed.source)
    except FileNotFoundError as exc:
        return structured_error(
            error_type="usage",
            subtype="bad_args",
            message=str(exc),
            hint="Pass an existing local path or a supported i讯飞 document URL.",
        )
    if parsed.as_json:
        print(json.dumps(payload, ensure_ascii=False))
    else:
        source_key = "sourceRef" if payload["remote"] else "source"
        print(f"source {payload[source_key]}")
        print(f"remote {str(payload['remote']).lower()}")
        print(f"kind {payload['kind']}")
        if payload["remote"]:
            print(f"host {payload['host']}")
            print(f"route {payload['route']}")
        else:
            print(f"path {payload['path']}")
            print(f"exists {str(payload['exists']).lower()}")
            print(f"readable {str(payload['readable']).lower()}")
    return 0


def run_docs_publish(args: list[str]) -> int:
    parser = argparse.ArgumentParser(prog="ixf docs publish")
    parser.add_argument("markdown")
    parser.add_argument("--base-url", required=True)
    parser.add_argument("--space-api", default=DEFAULT_SPACE_API)
    parser.add_argument("--cookies", default=DEFAULT_COOKIES)
    parser.add_argument("--member-id", default="")
    parser.add_argument("--parent-token", default="")
    parser.add_argument("--title", default="")
    parser.add_argument("--title-suffix", default="")
    parser.add_argument("--require", action="append", default=[])
    parser.add_argument("--apply", action="store_true")
    parser.add_argument("--dry-run", action="store_true")
    parsed = parser.parse_args(args)
    try:
        payload = publish_markdown(
            DocxPublishConfig(
                markdown_path=Path(parsed.markdown),
                base_url=parsed.base_url,
                cookies_path=Path(parsed.cookies),
                space_api=parsed.space_api,
                member_id=parsed.member_id,
                parent_token=parsed.parent_token,
                title=parsed.title,
                title_suffix=parsed.title_suffix,
                required_text=tuple(parsed.require),
                apply=bool(parsed.apply and not parsed.dry_run),
            )
        )
    except Exception as exc:
        return structured_error(
            error_type="remote",
            subtype="remote_write_failed",
            message=str(exc),
            hint="Check the Markdown path, tenant URL, local session, and document permissions.",
            retryable=True,
        )
    print(json.dumps(payload, ensure_ascii=False))
    return 0


def run_docs(args: list[str]) -> int:
    if not args:
        print("ERROR docs requires a subcommand.", file=sys.stderr)
        return 2
    command, rest = args[0], args[1:]
    if command == "read":
        return run_docs_read(rest)
    if command == "outline":
        return run_docs_outline(rest)
    if command == "chunk":
        return run_docs_chunk(rest)
    if command == "cleanup":
        return run_docs_cleanup(rest)
    if command == "inspect":
        return run_docs_inspect(rest)
    if command == "publish":
        return run_docs_publish(rest)
    print(f"ERROR unsupported docs subcommand: {command}", file=sys.stderr)
    return 2


def run_okr_read(args: list[str]) -> int:
    parser = argparse.ArgumentParser(prog="ixf okr read")
    parser.add_argument("source")
    parser.add_argument("--cookies", default=DEFAULT_COOKIES)
    parsed = parser.parse_args(args)
    try:
        result = read_okr_url(parsed.source, cookies_path=Path(parsed.cookies).expanduser())
    except FileNotFoundError as exc:
        return structured_error(
            error_type="cookie",
            subtype="cookie_file_missing",
            message=str(exc),
            hint="Run `ixf cookies export --provider auto --output <path>` or pass --cookies.",
        )
    except ValueError as exc:
        return structured_error(
            error_type="usage",
            subtype="bad_args",
            message=str(exc),
            hint="Pass a supported i讯飞 OKR page URL.",
        )
    except (requests.RequestException, RuntimeError) as exc:
        return structured_error(
            error_type="remote",
            subtype="remote_read_failed",
            message=str(exc),
            hint="Check network access, OKR permissions, and whether the local desktop session is still valid.",
            retryable=True,
        )
    sys.stdout.write(str(result["content"]))
    if not str(result["content"]).endswith("\n"):
        print()
    return 0


def run_okr_write(args: list[str]) -> int:
    parser = argparse.ArgumentParser(prog="ixf okr write")
    parser.add_argument("--url", required=True)
    parser.add_argument("--input", required=True)
    parser.add_argument("--cookies", default=DEFAULT_COOKIES)
    parser.add_argument("--base-url", default="")
    parser.add_argument("--objective-index", type=int)
    parser.add_argument("--prune", action="store_true")
    parser.add_argument("--apply", action="store_true")
    parser.add_argument("--dry-run", action="store_true")
    parsed = parser.parse_args(args)
    try:
        payload = write_okr(
            OkrWriteConfig(
                url=parsed.url,
                input_path=Path(parsed.input),
                cookies_path=Path(parsed.cookies),
                base_url=parsed.base_url,
                objective_index=parsed.objective_index,
                prune=parsed.prune,
                apply=bool(parsed.apply and not parsed.dry_run),
            )
        )
    except Exception as exc:
        return structured_error(
            error_type="remote",
            subtype="remote_write_failed",
            message=str(exc),
            hint="Check the OKR URL, input JSON, local session, and edit permissions.",
            retryable=True,
        )
    print(json.dumps(payload, ensure_ascii=False))
    return 0


def run_okr(args: list[str]) -> int:
    if not args:
        print("ERROR okr requires a subcommand.", file=sys.stderr)
        return 2
    command, rest = args[0], args[1:]
    if command == "read":
        return run_okr_read(rest)
    if command == "write":
        return run_okr_write(rest)
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
