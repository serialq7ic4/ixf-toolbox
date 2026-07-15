from __future__ import annotations

import hashlib
from http.server import BaseHTTPRequestHandler
import json
import os
from pathlib import Path
import subprocess
import sqlite3
import stat
from urllib.parse import parse_qs, urlparse

from go_poc_support import GO_ENV
from go_poc_support import ROOT
from go_poc_support import build_go_ixf
from go_poc_support import gzip_json
from go_poc_support import remote_docx_parity_client_vars
from go_poc_support import run_go_ixf
from go_poc_support import serve_handler
from go_poc_support import sheet_client_vars_payload
from go_poc_support import sheet_expansion_lines
from go_poc_support import write_cookie_fixture
from go_poc_support import write_json_response
from ixf_toolbox.core.docs.assets import has_valid_image_magic
from ixf_toolbox.core.docs.converters.docx_markdown import ConversionOptions
from ixf_toolbox.core.docs.converters.docx_markdown import ImageResolution
from ixf_toolbox.core.docs.converters.docx_markdown import convert_docx_client_vars


def test_go_ixf_version_matches_python_release(tmp_path):
    binary = build_go_ixf(tmp_path)
    result = run_go_ixf(binary, "--version")

    assert result.stdout.strip() == "ixf 2.6.0"
    assert result.stderr == ""


def test_go_ixf_helper_decodes_cli_output_as_utf8(monkeypatch, tmp_path):
    calls = []

    def fake_run(cmd, **kwargs):
        calls.append(kwargs)
        return subprocess.CompletedProcess(cmd, 0, stdout="提升平台稳定性\n", stderr="")

    monkeypatch.setattr(subprocess, "run", fake_run)

    result = run_go_ixf(tmp_path / "ixf-go", "okr", "read")

    assert result.stdout == "提升平台稳定性\n"
    assert calls[0]["encoding"] == "utf-8"


def test_go_ixf_root_help_writes_stdout_and_exits_zero(tmp_path):
    binary = build_go_ixf(tmp_path)
    result = run_go_ixf(binary, "--help")

    assert "usage: ixf [--version]" in result.stdout
    assert "docs" in result.stdout
    assert result.stderr == ""


def test_go_ixf_setup_skills_installs_embedded_codex_skills(tmp_path):
    binary = build_go_ixf(tmp_path)
    result = run_go_ixf(binary, "setup", "skills", "--runtimes", "codex", "--json", home=tmp_path)

    payload = json.loads(result.stdout)
    assert payload["ok"] is True
    assert len(payload["installed"]) == 5
    assert payload["skipped"] == []
    skill = tmp_path / ".codex" / "skills" / "using-ixf-toolbox" / "SKILL.md"
    assert skill.exists()
    assert "name: using-ixf-toolbox" in skill.read_text(encoding="utf-8")


def test_go_ixf_doctor_json_is_secret_safe_and_reports_go_runtime(tmp_path):
    binary = build_go_ixf(tmp_path)
    run_go_ixf(binary, "setup", "skills", "--runtimes", "codex", "--json", home=tmp_path)
    cookies = tmp_path / "cookies.json"
    write_cookie_fixture(cookies, csrf_token="dummy-csrf", session="dummy-session")

    result = run_go_ixf(binary, "doctor", "--cookies", str(cookies), "--json", home=tmp_path)
    payload = json.loads(result.stdout)
    serialized = json.dumps(payload, ensure_ascii=False)

    assert payload["ok"] is True
    assert payload["version"] == "2.6.0"
    assert payload["runtime"] == "go"
    assert payload["capabilities"]["cookiesExport"] is True
    assert payload["skills"]["codex"]["ok"] is True
    assert payload["cookies"]["hasCsrf"] is True
    assert "dummy-csrf" not in serialized
    assert "dummy-session" not in serialized


def create_chromium_cookie_db(path: Path) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    conn = sqlite3.connect(path)
    conn.execute(
        """
        CREATE TABLE cookies (
            host_key TEXT,
            name TEXT,
            value TEXT,
            encrypted_value BLOB,
            path TEXT,
            expires_utc INTEGER,
            is_secure INTEGER,
            is_httponly INTEGER,
            samesite INTEGER
        )
        """
    )
    conn.executemany(
        "INSERT INTO cookies VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
        [
            (
                ".example.test",
                "_csrf_token",
                "csrf-fixture",
                b"",
                "/",
                0,
                1,
                1,
                -1,
            ),
            (
                ".example.test",
                "session",
                "session-fixture",
                b"",
                "/",
                0,
                1,
                1,
                -1,
            ),
        ],
    )
    conn.commit()
    conn.close()


def test_go_ixf_cookies_export_reads_plain_chromium_cookie_db(tmp_path):
    binary = build_go_ixf(tmp_path)
    db = tmp_path / "profile" / "Cookies"
    create_chromium_cookie_db(db)
    output = tmp_path / "cookies.json"

    result = run_go_ixf(
        binary,
        "cookies",
        "export",
        "--provider",
        "macos-larkshell",
        "--cookies-db",
        str(db),
        "--host-like",
        "%.example.test%",
        "--output",
        str(output),
        "--json",
    )
    payload = json.loads(result.stdout)
    serialized = json.dumps(payload, ensure_ascii=False)
    cookies = json.loads(output.read_text(encoding="utf-8"))

    assert payload["ok"] is True
    assert payload["provider"] == "macos-larkshell"
    assert payload["cookieCount"] == 2
    assert payload["hasCsrf"] is True
    assert payload["output"] == str(output)
    assert [cookie["name"] for cookie in cookies] == ["_csrf_token", "session"]
    assert "csrf-fixture" not in serialized
    assert "session-fixture" not in serialized
    if os.name != "nt":
        assert stat.S_IMODE(output.stat().st_mode) == 0o600
    assert result.stderr == ""


def test_go_ixf_cookies_export_supports_windows_plain_cookie_db(tmp_path):
    binary = build_go_ixf(tmp_path)
    db = tmp_path / "LarkShell" / "User Data" / "Default" / "Network" / "Cookies"
    create_chromium_cookie_db(db)
    output = tmp_path / "cookies.json"

    result = run_go_ixf(
        binary,
        "cookies",
        "export",
        "--provider",
        "windows-larkshell",
        "--cookies-db",
        str(db),
        "--host-like",
        "%.example.test%",
        "--output",
        str(output),
        "--json",
    )
    payload = json.loads(result.stdout)
    cookies = json.loads(output.read_text(encoding="utf-8"))

    assert payload["ok"] is True
    assert payload["provider"] == "windows-larkshell"
    assert payload["cookieCount"] == 2
    assert payload["hasCsrf"] is True
    assert [cookie["value"] for cookie in cookies] == ["csrf-fixture", "session-fixture"]
    assert result.stderr == ""


def test_go_ixf_cookies_export_help_lists_provider_options(tmp_path):
    binary = build_go_ixf(tmp_path)

    result = run_go_ixf(binary, "cookies", "export", "--help")

    assert "Usage of ixf cookies export" in result.stdout
    assert "-cookies-db" in result.stdout
    assert "-host-like" in result.stdout
    assert "-keychain-service" in result.stdout
    assert "-local-state" in result.stdout
    assert result.stderr == ""


def test_go_ixf_docs_help_lists_local_v13_commands(tmp_path):
    binary = build_go_ixf(tmp_path)

    result = run_go_ixf(binary, "docs", "--help")

    assert "usage: ixf docs" in result.stdout
    assert "outline" in result.stdout
    assert "chunk" in result.stdout
    assert "inspect" in result.stdout
    assert "cleanup" in result.stdout
    assert "read" in result.stdout
    assert "publish" in result.stdout


def test_go_ixf_docs_publish_dry_run_prints_plan_without_cookie_file(tmp_path):
    binary = build_go_ixf(tmp_path)
    source = tmp_path / "notes.md"
    source.write_text(
        "# Dry Run\n\n"
        "Body with `inline code`.\n\n"
        "## Section\n\n"
        "- First bullet\n"
        "1. First ordered item\n\n"
        "```bash\n"
        "echo one\n"
        "echo two\n"
        "```\n",
        encoding="utf-8",
    )

    result = run_go_ixf(
        binary,
        "docs",
        "publish",
        str(source),
        "--base-url",
        "https://tenant.example.test",
        "--title-suffix",
        " - Draft",
        "--require",
        "Body",
        "--dry-run",
    )

    payload = json.loads(result.stdout)
    assert payload == {
        "ok": True,
        "dryRun": True,
        "title": "Dry Run - Draft",
        "counts": {
            "text": 1,
            "heading2": 1,
            "bullet": 1,
            "ordered": 1,
            "code": 1,
        },
    }
    assert result.stderr == ""


def test_go_ixf_docs_publish_apply_creates_writes_and_verifies_with_fixture_session(tmp_path):
    binary = build_go_ixf(tmp_path)
    source = tmp_path / "notes.md"
    source.write_text(
        "# Apply Title\n\n"
        "Body with required text.\n\n"
        "```bash\n"
        "echo one\n"
        "echo two\n"
        "```\n",
        encoding="utf-8",
    )
    cookies = tmp_path / "cookies.json"
    write_cookie_fixture(cookies)
    events = []

    class Handler(BaseHTTPRequestHandler):
        def do_POST(self):
            if self.path == "/space/api/explorer/v2/create/object/":
                length = int(self.headers.get("Content-Length", "0"))
                body = self.rfile.read(length).decode("utf-8")
                form = parse_qs(body)
                events.append(("create", form, self.headers.get("X-CSRFToken")))
                assert self.headers.get("X-CSRFToken") == "csrf-fixture"
                assert "session=session-fixture" in self.headers.get("Cookie", "")
                assert form["name"] == ["Apply Title - Published"]
                assert form["parent_token"] == ["parent_fixture"]
                write_json_response(
                    self,
                    {"code": 0, "data": {"obj_token": "doxrzCreatedPage"}},
                )
                return
            if self.path == "/space/api/docx/blocks/user_change":
                length = int(self.headers.get("Content-Length", "0"))
                payload = json.loads(self.rfile.read(length).decode("utf-8"))
                events.append(("write", payload, self.headers.get("X-CSRFToken")))
                assert self.headers.get("X-CSRFToken") == "csrf-fixture"
                assert "session=session-fixture" in self.headers.get("Cookie", "")
                assert payload["member_id"] == "member_override"
                assert payload["page_id"] == "doxrzCreatedPage"
                change_map = payload["change_map"]
                assert "doxrzCreatedPage" in change_map
                written_text = json.dumps(change_map, ensure_ascii=False)
                assert "Body with required text." in written_text
                assert "echo one\\necho two" in written_text
                write_json_response(self, {"code": 0, "data": {}})
                return
            self.send_error(404)

        def do_GET(self):
            parsed = urlparse(self.path)
            if parsed.path == "/space/api/docx/pages/client_vars":
                query = parse_qs(parsed.query)
                events.append(("client_vars", query, self.headers.get("X-CSRFToken")))
                assert self.headers.get("X-CSRFToken") == "csrf-fixture"
                assert query["id"] == ["doxrzCreatedPage"]
                if any(event[0] == "write" for event in events):
                    block_map = {
                        "doxrzCreatedPage": {
                            "version": 8,
                            "data": {
                                "type": "page",
                                "author": "author_fixture",
                                "children": ["text_1", "code_1"],
                            },
                        },
                        "text_1": {
                            "version": 1,
                            "data": {
                                "type": "text",
                                "text": {
                                    "initialAttributedTexts": {
                                        "text": {"0": "Body with required text."}
                                    }
                                },
                            },
                        },
                        "code_1": {
                            "version": 1,
                            "data": {
                                "type": "code",
                                "text": {
                                    "initialAttributedTexts": {
                                        "text": {"0": "echo one\necho two"}
                                    }
                                },
                            },
                        },
                    }
                else:
                    block_map = {
                        "doxrzCreatedPage": {
                            "version": 7,
                            "data": {
                                "type": "page",
                                "author": "author_fixture",
                                "children": [],
                            },
                        }
                    }
                write_json_response(self, {"code": 0, "data": {"block_map": block_map}})
                return
            self.send_error(404)

    with serve_handler(Handler) as server:
        result = run_go_ixf(
            binary,
            "docs",
            "publish",
            str(source),
            "--base-url",
            server,
            "--space-api",
            server,
            "--cookies",
            str(cookies),
            "--parent-token",
            "parent_fixture",
            "--member-id",
            "member_override",
            "--title-suffix",
            " - Published",
            "--require",
            "required text",
            "--apply",
        )

    payload = json.loads(result.stdout)
    assert payload["ok"] is True
    assert payload["dryRun"] is False
    assert payload["title"] == "Apply Title - Published"
    assert payload["url"].endswith("/docx/doxrzCreatedPage")
    assert payload["verify"]["ok"] is True
    assert [event[0] for event in events] == ["create", "client_vars", "write", "client_vars"]
    assert result.stderr == ""


def test_go_ixf_docs_read_prints_local_markdown_without_remote_session(tmp_path):
    binary = build_go_ixf(tmp_path)
    source = tmp_path / "source.md"
    source.write_text("# Source\n\nHello from local file.\n", encoding="utf-8")

    result = run_go_ixf(binary, "docs", "read", str(source))

    assert result.stdout == "# Source\n\nHello from local file.\n"
    assert result.stderr == ""


def test_go_ixf_docs_read_writes_manifest_and_can_cleanup(tmp_path):
    binary = build_go_ixf(tmp_path)
    source_a = tmp_path / "Project Plan.md"
    source_b = tmp_path / "project-plan.md"
    source_a.write_text("# A\n", encoding="utf-8")
    source_b.write_text("# B\n", encoding="utf-8")
    out_dir = tmp_path / "out"

    result = run_go_ixf(
        binary,
        "docs",
        "read",
        str(source_a),
        str(source_b),
        "--out-dir",
        str(out_dir),
        "--print-manifest",
    )
    manifest = json.loads(result.stdout)

    assert manifest["local_markdown_1"]["file"] == str(out_dir / "project-plan.md")
    assert manifest["local_markdown_2"]["file"] == str(out_dir / "project-plan-2.md")
    assert (out_dir / "project-plan.md").read_text(encoding="utf-8") == "# A\n"
    assert (out_dir / "project-plan-2.md").read_text(encoding="utf-8") == "# B\n"
    assert json.loads((out_dir / "manifest.json").read_text(encoding="utf-8")) == manifest

    run_go_ixf(
        binary,
        "docs",
        "read",
        str(source_a),
        "--out-dir",
        str(out_dir),
        "--cleanup",
    )

    assert not (out_dir / "manifest.json").exists()
    assert not (out_dir / "project-plan.md").exists()


def test_go_ixf_docs_read_remote_docx_uses_client_vars_api(tmp_path):
    binary = build_go_ixf(tmp_path)
    cookies = tmp_path / "cookies.json"
    write_cookie_fixture(cookies)
    errors: list[str] = []
    requested: list[str] = []

    class Handler(BaseHTTPRequestHandler):
        def do_GET(self) -> None:
            parsed = urlparse(self.path)
            requested.append(self.path)
            if parsed.path != "/space/api/docx/pages/client_vars":
                self.send_response(404)
                self.end_headers()
                return
            query = parse_qs(parsed.query)
            if self.headers.get("X-CSRFToken") != "csrf-fixture":
                errors.append("missing csrf header")
            if "session=session-fixture" not in self.headers.get("Cookie", ""):
                errors.append("missing session cookie")
            if query.get("id") != ["page_1"]:
                errors.append(f"unexpected id query: {query.get('id')}")
            if query.get("cursor") == ["next-cursor"]:
                payload = {
                    "code": 0,
                    "data": {
                        "block_map": {
                            "text_2": {
                                "data": {
                                    "type": "text",
                                    "parent_id": "page_1",
                                    "text": {"initialAttributedTexts": {"text": {"0": "Later"}}},
                                }
                            }
                        },
                        "has_more": False,
                    },
                }
            else:
                payload = {
                    "code": 0,
                    "data": {
                        "block_map": {
                            "page_1": {
                                "data": {
                                    "type": "page",
                                    "children": ["text_1", "text_2"],
                                    "text": {"initialAttributedTexts": {"text": {"0": "Remote Doc"}}},
                                }
                            },
                            "text_1": {
                                "data": {
                                    "type": "text",
                                    "parent_id": "page_1",
                                    "text": {"initialAttributedTexts": {"text": {"0": "First"}}},
                                }
                            },
                        },
                        "has_more": True,
                        "cursor": "next-cursor",
                    },
                }
            write_json_response(self, payload)

        def log_message(self, format: str, *args: object) -> None:
            return

    with serve_handler(Handler) as base_url:
        result = run_go_ixf(
            binary,
            "docs",
            "read",
            f"{base_url}/docx/page_1",
            "--cookies",
            str(cookies),
            "--space-api",
            base_url,
        )

    assert result.stdout == "# Remote Doc\n\nFirst\n\nLater\n"
    assert result.stderr == ""
    assert errors == []
    assert requested[-1] == "/space/api/docx/pages/client_vars?id=page_1&open_type=1&mode=4&cursor=next-cursor"


def test_go_ixf_docs_read_remote_wiki_resolves_docx_token_to_manifest(tmp_path):
    binary = build_go_ixf(tmp_path)
    cookies = tmp_path / "cookies.json"
    write_cookie_fixture(cookies)
    errors: list[str] = []
    requested: list[str] = []

    class Handler(BaseHTTPRequestHandler):
        def do_GET(self) -> None:
            parsed = urlparse(self.path)
            requested.append(self.path)
            if parsed.path == "/wiki/space/page":
                if self.headers.get("X-CSRFToken") != "csrf-fixture":
                    errors.append("missing wiki csrf header")
                if "session=session-fixture" not in self.headers.get("Cookie", ""):
                    errors.append("missing wiki session cookie")
                body = b'<script>window.__WIKI__={"obj_token":"page_1"}</script>'
                self.send_response(200)
                self.send_header("Content-Type", "text/html; charset=utf-8")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)
                return
            if parsed.path == "/space/api/docx/pages/client_vars":
                query = parse_qs(parsed.query)
                if query.get("id") != ["page_1"]:
                    errors.append(f"unexpected id query: {query.get('id')}")
                payload = {
                    "code": 0,
                    "data": {
                        "block_map": {
                            "page_1": {
                                "data": {
                                    "type": "page",
                                    "children": ["text_1"],
                                    "text": {"initialAttributedTexts": {"text": {"0": "Wiki Doc"}}},
                                }
                            },
                            "text_1": {
                                "data": {
                                    "type": "text",
                                    "parent_id": "page_1",
                                    "text": {"initialAttributedTexts": {"text": {"0": "Resolved body"}}},
                                }
                            },
                        },
                        "has_more": False,
                    },
                }
                write_json_response(self, payload)
                return
            self.send_response(404)
            self.end_headers()

        def log_message(self, format: str, *args: object) -> None:
            return

    out_dir = tmp_path / "out"
    with serve_handler(Handler) as base_url:
        result = run_go_ixf(
            binary,
            "docs",
            "read",
            f"{base_url}/wiki/space/page?from=copy",
            "--cookies",
            str(cookies),
            "--space-api",
            base_url,
            "--out-dir",
            str(out_dir),
            "--print-manifest",
        )

    manifest = json.loads(result.stdout)
    item = manifest["wiki_1"]
    assert Path(item["file"]).read_text(encoding="utf-8") == "# Wiki Doc\n\nResolved body\n"
    assert item["kind"] == "wiki"
    assert item["token"] == "page_1"
    assert errors == []
    assert requested[0] == "/wiki/space/page?from=copy"
    assert requested[-1] == "/space/api/docx/pages/client_vars?id=page_1&open_type=1"
    assert result.stderr == ""


def test_go_ixf_docs_read_remote_wiki_bitable_renders_tsv_manifest(tmp_path):
    binary = build_go_ixf(tmp_path)
    cookies = tmp_path / "cookies.json"
    write_cookie_fixture(cookies)
    errors: list[str] = []
    requested: list[str] = []
    old_schema = {
        "base": {
            "name": "Team Tracker",
            "timezone": "UTC",
            "tables": ["tbl1"],
            "tableInfos": {"tbl1": {"name": "Projects"}},
        },
        "data": {
            "table": {
                "views": ["view_grid"],
                "viewMap": {
                    "view_grid": {
                        "id": "view_grid",
                        "name": "Grid View",
                        "type": 1,
                        "property": {
                            "fields": ["fld_name", "fld_status", "fld_owner", "fld_due"],
                            "records": ["rec1", "rec2"],
                        },
                    }
                },
                "fieldMap": {
                    "fld_name": {"name": "Name", "type": 1},
                    "fld_status": {
                        "name": "Status",
                        "type": 3,
                        "property": {
                            "options": [
                                {"id": "todo", "name": "To Do"},
                                {"id": "done", "name": "Done"},
                            ]
                        },
                    },
                    "fld_owner": {"name": "Owner", "type": 11},
                    "fld_due": {
                        "name": "Due",
                        "type": 5,
                        "property": {"dateFormat": "yyyy/MM/dd", "timeFormat": "HH:mm"},
                    },
                },
            },
            "recordMap": {
                "rec1": {
                    "fld_name": {"value": "Alpha"},
                    "fld_status": {"value": "todo"},
                    "fld_owner": {"value": ["u1"]},
                    "fld_due": {"value": 0},
                },
                "rec2": {
                    "fld_name": {"value": "Beta\tLine"},
                    "fld_status": {"value": "done"},
                    "fld_owner": {"value": []},
                    "fld_due": {"value": None},
                },
            },
        },
    }

    class Handler(BaseHTTPRequestHandler):
        def do_GET(self) -> None:
            parsed = urlparse(self.path)
            requested.append(self.path)
            if parsed.path == "/wiki/base/page":
                if self.headers.get("X-CSRFToken") != "csrf-fixture":
                    errors.append("missing wiki csrf header")
                if "session=session-fixture" not in self.headers.get("Cookie", ""):
                    errors.append("missing wiki session cookie")
                wiki_info = {"obj_token": "base_1"}
                body = (
                    "<script>window.wiki_suite_type = 'bitable';"
                    "current_space_wiki = Object("
                    + json.dumps(wiki_info, ensure_ascii=False)
                    + ")</script>"
                ).encode("utf-8")
                self.send_response(200)
                self.send_header("Content-Type", "text/html; charset=utf-8")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)
                return
            if parsed.path == "/space/api/v1/bitable/base_1/clientvars":
                query = parse_qs(parsed.query)
                if query.get("recordLimit") != ["2000"]:
                    errors.append(f"unexpected bitable query: {parsed.query}")
                if self.headers.get("Referer") != f"http://{self.headers.get('Host')}/wiki/base/page?from=copy":
                    errors.append("unexpected bitable referer")
                payload = {
                    "code": 0,
                    "data": {
                        "oldSchema": {"gzipSchema": gzip_json(old_schema)},
                        "users": {"u1": {"name": "Alice"}},
                    },
                }
                write_json_response(self, payload)
                return
            self.send_response(404)
            self.end_headers()

        def log_message(self, format: str, *args: object) -> None:
            return

    out_dir = tmp_path / "out"
    with serve_handler(Handler) as base_url:
        result = run_go_ixf(
            binary,
            "docs",
            "read",
            f"{base_url}/wiki/base/page?from=copy",
            "--cookies",
            str(cookies),
            "--out-dir",
            str(out_dir),
            "--print-manifest",
        )

    manifest = json.loads(result.stdout)
    item = manifest["wiki_bitable_1"]
    assert Path(item["file"]).read_text(encoding="utf-8") == (
        "# Team Tracker\n\n"
        "[bitable token=base_1]\n"
        '[bitable-meta base_token=base_1 table_id=tbl1 table_name="Projects" '
        'view_id=view_grid view_name="Grid View" rows=2 cols=4 views=1]\n'
        "```tsv\n"
        "Name\tStatus\tOwner\tDue\n"
        "Alpha\tTo Do\tAlice\t1970/01/01 00:00\n"
        "Beta\\tLine\tDone\n"
        "```\n"
    )
    assert item["kind"] == "wiki_bitable"
    assert item["title"] == "Team Tracker"
    assert item["token"] == "base_1"
    assert item["counts"] == {
        "bitable": 1,
        "bitable_fields": 4,
        "bitable_records": 2,
        "bitable_views": 1,
    }
    assert item["assets"] == []
    assert item["warnings"] == []
    assert errors == []
    assert requested[0] == "/wiki/base/page?from=copy"
    assert requested[1].startswith("/space/api/v1/bitable/base_1/clientvars?")
    assert result.stderr == ""


def test_go_ixf_docs_read_remote_mindnote_matches_python_contract(tmp_path):
    binary = build_go_ixf(tmp_path)
    cookies = tmp_path / "cookies.json"
    write_cookie_fixture(cookies)
    errors: list[str] = []
    requested: list[str] = []

    class Handler(BaseHTTPRequestHandler):
        def do_GET(self) -> None:
            parsed = urlparse(self.path)
            requested.append(self.path)
            if parsed.path != "/mindnotes/mind_1":
                self.send_response(404)
                self.end_headers()
                return
            if self.headers.get("X-CSRFToken") != "csrf-fixture":
                errors.append("missing csrf header")
            if "session=session-fixture" not in self.headers.get("Cookie", ""):
                errors.append("missing session cookie")
            payload = {
                "token": "mind_1",
                "data": {
                    "title": "Q3 Mindnote",
                    "collab_client_vars": {
                        "nodes": [
                            {
                                "text": [{"text": "Root"}],
                                "children": [{"text": [{"text": "Child"}], "children": []}],
                            },
                            {"text": [{"text": "Second"}], "children": []},
                        ]
                    },
                },
            }
            body = (
                "<html><script>window.bootstrap={clientVars: Object("
                + json.dumps(payload, ensure_ascii=False)
                + ")}</script></html>"
            ).encode("utf-8")
            self.send_response(200)
            self.send_header("Content-Type", "text/html; charset=utf-8")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def log_message(self, format: str, *args: object) -> None:
            return

    out_dir = tmp_path / "out"
    with serve_handler(Handler) as base_url:
        result = run_go_ixf(
            binary,
            "docs",
            "read",
            f"{base_url}/mindnotes/mind_1?from=copy",
            "--cookies",
            str(cookies),
            "--out-dir",
            str(out_dir),
            "--print-manifest",
        )

    manifest = json.loads(result.stdout)
    item = manifest["mindnote_1"]
    assert Path(item["file"]).read_text(encoding="utf-8") == (
        "# Q3 Mindnote\n\n- Root\n  - Child\n- Second\n"
    )
    assert item["kind"] == "mindnote"
    assert item["title"] == "Q3 Mindnote"
    assert item["token"] == "mind_1"
    assert item["counts"] == {"mindnote_nodes": 2}
    assert item["assets"] == []
    assert item["warnings"] == []
    assert errors == []
    assert requested == ["/mindnotes/mind_1?from=copy"]
    assert result.stderr == ""


def test_go_ixf_docs_read_remote_docx_downloads_images_to_manifest(tmp_path):
    binary = build_go_ixf(tmp_path)
    cookies = tmp_path / "cookies.json"
    write_cookie_fixture(cookies)
    image_token = "boxr-image-token"
    png_bytes = b"\x89PNG\r\n\x1a\n" + b"\x00" * 64
    errors: list[str] = []

    class Handler(BaseHTTPRequestHandler):
        def do_GET(self) -> None:
            parsed = urlparse(self.path)
            if parsed.path == "/space/api/docx/pages/client_vars":
                payload = {
                    "code": 0,
                    "data": {
                        "block_map": {
                            "page_1": {
                                "data": {
                                    "type": "page",
                                    "children": ["image_1"],
                                    "text": {"initialAttributedTexts": {"text": {"0": "Image Doc"}}},
                                }
                            },
                            "image_1": {
                                "data": {
                                    "type": "image",
                                    "parent_id": "page_1",
                                    "image": {
                                        "token": image_token,
                                        "name": "architecture.png",
                                        "mimeType": "image/png",
                                        "width": 1200,
                                        "height": 800,
                                        "size": len(png_bytes),
                                        "caption": {
                                            "initialAttributedTexts": {"text": {"0": "Architecture diagram"}}
                                        },
                                    },
                                }
                            },
                        },
                        "has_more": False,
                    },
                }
                write_json_response(self, payload)
                return
            if parsed.path == f"/space/api/box/stream/download/all/{image_token}/":
                query = parse_qs(parsed.query)
                if query.get("mount_node_token") != ["page_1"]:
                    errors.append("missing mount_node_token")
                if query.get("mount_point") != ["docx_image"]:
                    errors.append("missing mount_point")
                if self.headers.get("X-CSRFToken") != "csrf-fixture":
                    errors.append("missing csrf header")
                body = png_bytes
                self.send_response(200)
                self.send_header("Content-Type", "image/png")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)
                return
            self.send_response(404)
            self.end_headers()

        def log_message(self, format: str, *args: object) -> None:
            return

    out_dir = tmp_path / "out"
    with serve_handler(Handler) as base_url:
        result = run_go_ixf(
            binary,
            "docs",
            "read",
            f"{base_url}/docx/page_1",
            "--cookies",
            str(cookies),
            "--space-api",
            base_url,
            "--out-dir",
            str(out_dir),
            "--download-images",
            "--print-manifest",
        )

    manifest = json.loads(result.stdout)
    item = manifest["docx_1"]
    markdown_path = Path(item["file"])
    assert markdown_path.read_text(encoding="utf-8") == (
        "# Image Doc\n\n![Architecture diagram](assets/docx_1/image-001.png)\n"
    )
    assert item["assets"] == [
        {
            "path": "assets/docx_1/image-001.png",
            "mimeType": "image/png",
            "width": 1200,
            "height": 800,
            "sizeBytes": len(png_bytes),
            "status": "downloaded",
            "ordinal": 1,
        }
    ]
    assert (out_dir / "assets" / "docx_1" / "image-001.png").read_bytes() == png_bytes
    assert errors == []
    serialized = json.dumps(manifest, ensure_ascii=False)
    assert image_token not in serialized
    assert result.stderr == ""


def test_go_ixf_docs_read_remote_docx_expands_sheets_to_manifest(tmp_path):
    binary = build_go_ixf(tmp_path)
    cookies = tmp_path / "cookies.json"
    write_cookie_fixture(cookies)
    errors: list[str] = []

    class Handler(BaseHTTPRequestHandler):
        def do_GET(self) -> None:
            parsed = urlparse(self.path)
            if parsed.path != "/space/api/docx/pages/client_vars":
                self.send_response(404)
                self.end_headers()
                return
            payload = {
                "code": 0,
                "data": {
                    "block_map": {
                        "page_1": {
                            "data": {
                                "type": "page",
                                "children": ["sheet_1"],
                                "text": {"initialAttributedTexts": {"text": {"0": "Sheet Doc"}}},
                            }
                        },
                        "sheet_1": {
                            "data": {
                                "type": "sheet",
                                "parent_id": "page_1",
                                "token": "shtr_fixture_sheet1",
                            }
                        },
                    },
                    "has_more": False,
                },
            }
            write_json_response(self, payload)

        def do_POST(self) -> None:
            parsed = urlparse(self.path)
            if parsed.path != "/space/api/v3/sheet/client_vars":
                self.send_response(404)
                self.end_headers()
                return
            query = parse_qs(parsed.query)
            if query.get("synced_block_host_token") != ["page_1"]:
                errors.append("missing synced_block_host_token")
            if self.headers.get("X-CSRFToken") != "csrf-fixture":
                errors.append("missing csrf header")
            request = json.loads(self.rfile.read(int(self.headers.get("Content-Length", "0"))))
            if request["token"] != "shtr_fixture":
                errors.append("wrong workbook token")
            if request["sheetRange"]["sheetId"] != "sheet1":
                errors.append("wrong sheet id")
            write_json_response(self, sheet_client_vars_payload())

        def log_message(self, format: str, *args: object) -> None:
            return

    out_dir = tmp_path / "out"
    with serve_handler(Handler) as base_url:
        result = run_go_ixf(
            binary,
            "docs",
            "read",
            f"{base_url}/docx/page_1",
            "--cookies",
            str(cookies),
            "--space-api",
            base_url,
            "--out-dir",
            str(out_dir),
            "--expand-sheets",
            "--print-manifest",
        )

    manifest = json.loads(result.stdout)
    item = manifest["docx_1"]
    markdown_path = Path(item["file"])
    assert markdown_path.read_text(encoding="utf-8") == (
        "# Sheet Doc\n\n"
        "[sheet token=shtr_fixture_sheet1]\n"
        "[sheet-meta workbook_token=shtr_fixture sheet_id=sheet1 rows=2 cols=2]\n"
        "```tsv\n"
        "Name\tValue\n"
        "Alpha\t42\n"
        "```\n"
    )
    assert item["counts"]["sheet_expanded"] == 1
    assert errors == []
    assert result.stderr == ""


def test_go_ixf_docs_read_remote_docx_matches_python_golden_for_mixed_blocks(tmp_path):
    binary = build_go_ixf(tmp_path)
    cookies = tmp_path / "cookies.json"
    write_cookie_fixture(cookies)
    image_token = "boxr-invalid-svg-token"
    invalid_svg = b"<html>not svg</html>"
    client_vars_data = remote_docx_parity_client_vars(image_token)
    errors: list[str] = []

    def python_image_reference(_reference) -> ImageResolution:
        assert not has_valid_image_magic("image/svg+xml", invalid_svg)
        return ImageResolution(
            markdown_path=None,
            alt_text="Architecture diagram",
            warning="image 1 download failed: content_error",
        )

    expected_conversion = convert_docx_client_vars(
        client_vars_data,
        "page_1",
        ConversionOptions(
            expand_sheet=lambda _token: sheet_expansion_lines(),
            resolve_image=python_image_reference,
        ),
    )
    expected_counts = dict(expected_conversion.counts)
    expected_counts["sheet_expanded"] = 1

    class Handler(BaseHTTPRequestHandler):
        def do_GET(self) -> None:
            parsed = urlparse(self.path)
            if parsed.path == "/space/api/docx/pages/client_vars":
                query = parse_qs(parsed.query)
                if query.get("id") != ["page_1"]:
                    errors.append("wrong docx id")
                if self.headers.get("X-CSRFToken") != "csrf-fixture":
                    errors.append("missing csrf header")
                if "session=session-fixture" not in self.headers.get("Cookie", ""):
                    errors.append("missing session cookie")
                write_json_response(self, {"code": 0, "data": {**client_vars_data, "has_more": False}})
                return
            if parsed.path == f"/space/api/box/stream/download/all/{image_token}/":
                query = parse_qs(parsed.query)
                if query.get("mount_node_token") != ["page_1"]:
                    errors.append("missing mount_node_token")
                if query.get("mount_point") != ["docx_image"]:
                    errors.append("missing mount_point")
                body = invalid_svg
                self.send_response(200)
                self.send_header("Content-Type", "image/svg+xml")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)
                return
            self.send_response(404)
            self.end_headers()

        def do_POST(self) -> None:
            parsed = urlparse(self.path)
            if parsed.path != "/space/api/v3/sheet/client_vars":
                self.send_response(404)
                self.end_headers()
                return
            query = parse_qs(parsed.query)
            if query.get("synced_block_host_token") != ["page_1"]:
                errors.append("missing synced_block_host_token")
            request = json.loads(self.rfile.read(int(self.headers.get("Content-Length", "0"))))
            if request["token"] != "shtr_fixture":
                errors.append("wrong workbook token")
            if request["sheetRange"]["sheetId"] != "sheet1":
                errors.append("wrong sheet id")
            write_json_response(self, sheet_client_vars_payload())

        def log_message(self, format: str, *args: object) -> None:
            return

    out_dir = tmp_path / "out"
    with serve_handler(Handler) as base_url:
        result = run_go_ixf(
            binary,
            "docs",
            "read",
            f"{base_url}/docx/page_1",
            "--cookies",
            str(cookies),
            "--space-api",
            base_url,
            "--out-dir",
            str(out_dir),
            "--download-images",
            "--expand-sheets",
            "--print-manifest",
        )

    manifest = json.loads(result.stdout)
    item = manifest["docx_1"]
    markdown_path = Path(item["file"])
    assert markdown_path.read_text(encoding="utf-8") == expected_conversion.markdown
    assert item["counts"] == expected_counts
    assert item["assets"] == []
    assert item["warnings"] == expected_conversion.warnings
    assert not (out_dir / "assets" / "docx_1" / "image-001.svg").exists()
    assert image_token not in json.dumps(manifest, ensure_ascii=False)
    assert errors == []
    assert result.stderr == ""


def test_go_ixf_docs_outline_and_chunk_match_local_markdown_contract(tmp_path):
    binary = build_go_ixf(tmp_path)
    source = tmp_path / "source.md"
    source.write_text(
        "# Title\n\n## One\n\nAlpha\n\n## Two\n\n![Diagram](assets/docx_1/image-001.png)\n*Caption*\n",
        encoding="utf-8",
    )

    outline_result = run_go_ixf(
        binary,
        "docs",
        "outline",
        str(source),
        "--target-chars",
        "40",
        "--json",
    )
    outline = json.loads(outline_result.stdout)

    assert outline["ok"] is True
    assert outline["selectedHeadingLevel"] == 2
    breadcrumbs = [chunk["breadcrumb"] for chunk in outline["chunks"]]
    assert "Title > One" in breadcrumbs
    assert "Title > Two" in breadcrumbs
    assert outline["chunks"][-1]["imagePaths"] == ["assets/docx_1/image-001.png"]

    chunk_result = run_go_ixf(
        binary,
        "docs",
        "chunk",
        str(source),
        "--index",
        "2",
        "--target-chars",
        "40",
    )

    assert chunk_result.stdout.startswith('[chunk 2/')
    assert "Title > One" in chunk_result.stdout
    assert "Alpha" in chunk_result.stdout
    assert "Diagram" not in chunk_result.stdout


def test_go_ixf_docs_inspect_is_secret_safe_for_local_and_remote_sources(tmp_path):
    binary = build_go_ixf(tmp_path)
    source = tmp_path / "private-source.md"
    source.write_text("# Secret Title\n\nSensitive body should not appear.\n", encoding="utf-8")

    local_result = run_go_ixf(binary, "docs", "inspect", str(source), "--json")
    local = json.loads(local_result.stdout)
    local_serialized = json.dumps(local, ensure_ascii=False)

    assert local["kind"] == "local_markdown"
    assert local["remote"] is False
    assert local["sizeBytes"] == source.stat().st_size
    assert "Secret Title" not in local_serialized
    assert "Sensitive body" not in local_serialized

    remote_result = run_go_ixf(
        binary,
        "docs",
        "inspect",
        "https://tenant.example.test/docx/doxfixturetoken?from=copy",
        "--json",
    )
    remote = json.loads(remote_result.stdout)
    remote_serialized = json.dumps(remote, ensure_ascii=False)

    assert remote["kind"] == "docx"
    assert remote["sourceRef"] == "https://tenant.example.test/docx/<redacted>?from=copy"
    assert remote["tokenPrefix"] == "dox"
    assert remote["tokenLength"] == len("doxfixturetoken")
    assert "doxfixturetoken" not in remote_serialized


def test_go_ixf_docs_read_rejects_okr_url_before_cookie_loading(tmp_path):
    binary = build_go_ixf(tmp_path)

    result = run_go_ixf(
        binary,
        "docs",
        "read",
        "https://tenant.example.test/okr/user/owner-fixture/?okrId=okr-fixture-200",
        check=False,
    )

    assert result.returncode == 2
    assert result.stdout == ""
    assert "docs read does not support OKR URLs" in result.stderr
    assert "ixf okr read" in result.stderr
    assert "cookie file" not in result.stderr
    assert "okr-fixture-200" not in result.stderr


def test_go_ixf_okr_help_lists_read_and_write(tmp_path):
    binary = build_go_ixf(tmp_path)

    result = run_go_ixf(binary, "okr", "--help")

    assert "usage: ixf okr" in result.stdout
    assert "read" in result.stdout
    assert "write" in result.stdout
    assert result.stderr == ""


def test_go_ixf_okr_read_uses_lgw_csrf_and_renders_markdown(tmp_path):
    binary = build_go_ixf(tmp_path)
    cookies = tmp_path / "cookies.json"
    write_cookie_fixture(cookies)
    events = []

    class Handler(BaseHTTPRequestHandler):
        def do_GET(self):
            parsed = urlparse(self.path)
            if parsed.path == "/lgw/csrf_token":
                events.append(("csrf", self.headers.get("Cookie", "")))
                self.send_response(200)
                self.send_header("Set-Cookie", "lgw_csrf_token=lgw-fixture; Path=/")
                self.send_header("Content-Length", "2")
                self.end_headers()
                self.wfile.write(b"{}")
                return
            if parsed.path == "/okrx/api/okr/owner/aggr_detail/":
                query = parse_qs(parsed.query)
                events.append(("detail", query, self.headers.get("x-lgw-csrf-token")))
                assert query["okr_id"] == ["okr-fixture-200"]
                assert query["withoutAddVisitLog"] == ["true"]
                assert self.headers.get("x-lgw-csrf-token") == "lgw-fixture"
                assert "session=session-fixture" in self.headers.get("Cookie", "")
                write_json_response(
                    self,
                    {
                        "code": 0,
                        "okr_detail_data": {
                            "name": "2026 Q3",
                            "owner_info": {
                                "user_info": {"locale_names": {"zh": "Fixture Owner"}}
                            },
                            "objective_list": [
                                {
                                    "id": "o1",
                                    "name": {"blocks": [{"text": "提升平台稳定性"}]},
                                    "kr_list": [
                                        {
                                            "id": "kr1",
                                            "content": {"blocks": [{"text": "完成可观测治理"}]},
                                            "progress_rate": {"percent": 20},
                                        },
                                        {
                                            "id": "kr2",
                                            "content_v2": {
                                                "0": {
                                                    "ops": [
                                                        {"insert": "完成容量巡检自动化\n"}
                                                    ]
                                                }
                                            },
                                        },
                                    ],
                                }
                            ],
                        },
                    },
                )
                return
            self.send_error(404)

    with serve_handler(Handler) as server:
        result = run_go_ixf(
            binary,
            "okr",
            "read",
            f"{server}/okr/user/owner-fixture/?okrId=okr-fixture-200&type=leader",
            "--cookies",
            str(cookies),
            "--csrf-url",
            f"{server}/lgw/csrf_token",
        )

    assert "# OKR - Fixture Owner - 2026 Q3" in result.stdout
    assert "[okr id=okr-fixture-200 objectives=1]" in result.stdout
    assert "## O1 提升平台稳定性" in result.stdout
    assert "- KR1: 完成可观测治理 _(progress: 20%)_" in result.stdout
    assert "- KR2: 完成容量巡检自动化" in result.stdout
    assert [event[0] for event in events] == ["csrf", "detail"]
    assert result.stderr == ""


def test_go_ixf_okr_write_dry_run_validates_input_without_cookie_file(tmp_path):
    binary = build_go_ixf(tmp_path)
    source = tmp_path / "okr.json"
    source.write_text(
        json.dumps(
            {
                "objectives": [
                    {
                        "objective": "Improve platform reliability",
                        "krs": [
                            "Complete the first measurable reliability milestone.",
                            "Complete the second measurable reliability milestone.",
                            "Complete the third measurable reliability milestone.",
                        ],
                    }
                ]
            }
        ),
        encoding="utf-8",
    )

    result = run_go_ixf(
        binary,
        "okr",
        "write",
        "--url",
        "https://tenant.example.test/okr/user/example/?okrId=example-okr",
        "--input",
        str(source),
        "--objective-index",
        "3",
        "--dry-run",
    )

    payload = json.loads(result.stdout)
    assert payload == {
        "ok": True,
        "dryRun": True,
        "okrId": "example-okr",
        "targetObjectiveIndex": 3,
        "objectives": [
            {
                "index": 1,
                "objective": "Improve platform reliability",
                "krCount": 3,
                "action": "plan",
            }
        ],
        "applySupported": True,
    }
    assert result.stderr == ""


def test_go_ixf_okr_write_apply_updates_target_objective_by_index(tmp_path):
    binary = build_go_ixf(tmp_path)
    source = tmp_path / "okr.json"
    source.write_text(
        json.dumps(
            {
                "objectives": [
                    {
                        "objective": "New O3",
                        "krs": ["New KR1", "New KR2"],
                    }
                ]
            }
        ),
        encoding="utf-8",
    )
    cookies = tmp_path / "cookies.json"
    write_cookie_fixture(cookies)
    events = []

    def detail_payload(final=False):
        o3 = {
            "id": "o3",
            "name": {"blocks": [{"text": "New O3" if final else "Old O3"}]},
            "kr_list": (
                [
                    {"id": "new-kr-1", "content": {"blocks": [{"text": "New KR1"}]}},
                    {"id": "new-kr-2", "content": {"blocks": [{"text": "New KR2"}]}},
                ]
                if final
                else [{"id": "old-kr", "content": {"blocks": [{"text": "Old KR"}]}}]
            ),
        }
        return {
            "code": 0,
            "okr_detail_data": {
                "name": "2026 Q3",
                "objective_list": [
                    {"id": "o1", "name": {"blocks": [{"text": "O1"}]}, "kr_list": []},
                    {"id": "o2", "name": {"blocks": [{"text": "O2"}]}, "kr_list": []},
                    o3,
                ],
            },
        }

    class Handler(BaseHTTPRequestHandler):
        def do_GET(self):
            parsed = urlparse(self.path)
            if parsed.path == "/lgw/csrf_token":
                events.append(("csrf", "GET", parsed.path))
                self.send_response(200)
                self.send_header("Set-Cookie", "lgw_csrf_token=lgw-fixture; Path=/")
                self.send_header("Content-Length", "2")
                self.end_headers()
                self.wfile.write(b"{}")
                return
            if parsed.path == "/okrx/api/okr/owner/aggr_detail/":
                events.append(("detail", "GET", parsed.path))
                assert self.headers.get("x-lgw-csrf-token") == "lgw-fixture"
                write_json_response(
                    self,
                    detail_payload(final=any(event[0] == "publish" for event in events)),
                )
                return
            if parsed.path == "/okrx/api/okr/example-okr/version/":
                events.append(("version", "GET", parsed.path))
                write_json_response(self, {"code": 0, "data": {"okr_draft_version": "version-1"}})
                return
            self.send_error(404)

        def do_POST(self):
            parsed = urlparse(self.path)
            length = int(self.headers.get("Content-Length", "0"))
            payload = json.loads(self.rfile.read(length).decode("utf-8") or "{}")
            if parsed.path == "/okrx/api/draft_v2/enable/o3/":
                events.append(("enable", "POST", parsed.path))
                assert payload["draft_version"] == "version-1"
                write_json_response(self, {"code": 0, "data": {"draft_version": "version-2"}})
                return
            if parsed.path == "/okrx/api/draft_v2/kr/":
                index = 1 + len([event for event in events if event[0] == "create_kr"])
                events.append(("create_kr", "POST", parsed.path))
                assert payload["objective_id"] == "o3"
                write_json_response(
                    self,
                    {"code": 0, "data": {"kr_id": f"new-kr-{index}", "draft_version": "version-4"}},
                )
                return
            if parsed.path == "/okrx/api/draft_v2/publish/o3/":
                events.append(("publish", "POST", parsed.path))
                assert payload["need_delete_kr_ids"] == ["old-kr"]
                assert payload["auto_notify"] is False
                write_json_response(self, {"code": 0, "data": {"draft_version": "version-9"}})
                return
            self.send_error(404)

        def do_PUT(self):
            parsed = urlparse(self.path)
            length = int(self.headers.get("Content-Length", "0"))
            payload = json.loads(self.rfile.read(length).decode("utf-8") or "{}")
            if parsed.path == "/okrx/api/draft_v2/objective/o3/":
                events.append(("objective", "PUT", parsed.path))
                assert "New O3" in payload["name"]
                write_json_response(self, {"code": 0, "data": {"draft_version": "version-3"}})
                return
            if parsed.path in {
                "/okrx/api/draft_v2/kr/new-kr-1/",
                "/okrx/api/draft_v2/kr/new-kr-2/",
            }:
                events.append(("kr_text", "PUT", parsed.path))
                assert "New KR" in payload["content"]
                write_json_response(self, {"code": 0, "data": {"draft_version": "version-5"}})
                return
            self.send_error(404)

        def do_DELETE(self):
            parsed = urlparse(self.path)
            if parsed.path == "/okrx/api/draft_v2/kr/old-kr/":
                events.append(("delete_kr", "DELETE", parsed.path))
                write_json_response(self, {"code": 0, "data": {"draft_version": "version-6"}})
                return
            self.send_error(404)

    with serve_handler(Handler) as server:
        result = run_go_ixf(
            binary,
            "okr",
            "write",
            "--url",
            f"{server}/okr/user/example/?okrId=example-okr",
            "--input",
            str(source),
            "--objective-index",
            "3",
            "--cookies",
            str(cookies),
            "--csrf-url",
            f"{server}/lgw/csrf_token",
            "--apply",
        )

    payload = json.loads(result.stdout)
    assert payload["ok"] is True
    assert payload["dryRun"] is False
    assert payload["target"] == {
        "objective": "New O3",
        "krs": [{"text": "New KR1"}, {"text": "New KR2"}],
    }
    assert [event[0] for event in events] == [
        "csrf",
        "detail",
        "version",
        "enable",
        "objective",
        "delete_kr",
        "create_kr",
        "kr_text",
        "create_kr",
        "kr_text",
        "publish",
        "detail",
    ]
    assert result.stderr == ""


def test_go_ixf_okr_write_apply_creates_next_objective_by_index(tmp_path):
    binary = build_go_ixf(tmp_path)
    source = tmp_path / "okr.json"
    source.write_text(
        json.dumps(
            {
                "objectives": [
                    {
                        "objective": "Created O3",
                        "krs": ["Created KR1", "Created KR2"],
                    }
                ]
            }
        ),
        encoding="utf-8",
    )
    cookies = tmp_path / "cookies.json"
    write_cookie_fixture(cookies)
    events = []

    def detail_payload(final=False):
        objectives = [
            {"id": "o1", "name": {"blocks": [{"text": "O1"}]}, "kr_list": []},
            {"id": "o2", "name": {"blocks": [{"text": "O2"}]}, "kr_list": []},
        ]
        if final:
            objectives.append(
                {
                    "id": "new-o3",
                    "name": {"blocks": [{"text": "Created O3"}]},
                    "kr_list": [
                        {"id": "new-kr-1", "content": {"blocks": [{"text": "Created KR1"}]}},
                        {"id": "new-kr-2", "content": {"blocks": [{"text": "Created KR2"}]}},
                    ],
                }
            )
        return {
            "code": 0,
            "okr_detail_data": {
                "name": "2026 Q3",
                "objective_list": objectives,
            },
        }

    class Handler(BaseHTTPRequestHandler):
        def do_GET(self):
            parsed = urlparse(self.path)
            if parsed.path == "/lgw/csrf_token":
                events.append(("csrf", "GET", parsed.path))
                self.send_response(200)
                self.send_header("Set-Cookie", "lgw_csrf_token=lgw-fixture; Path=/")
                self.send_header("Content-Length", "2")
                self.end_headers()
                self.wfile.write(b"{}")
                return
            if parsed.path == "/okrx/api/okr/owner/aggr_detail/":
                events.append(("detail", "GET", parsed.path))
                write_json_response(
                    self,
                    detail_payload(final=any(event[0] == "publish" for event in events)),
                )
                return
            if parsed.path == "/okrx/api/okr/example-okr/version/":
                events.append(("version", "GET", parsed.path))
                write_json_response(self, {"code": 0, "data": {"okr_draft_version": "version-1"}})
                return
            self.send_error(404)

        def do_POST(self):
            parsed = urlparse(self.path)
            length = int(self.headers.get("Content-Length", "0"))
            payload = json.loads(self.rfile.read(length).decode("utf-8") or "{}")
            if parsed.path == "/okrx/api/draft_v2/objective/":
                events.append(("create_objective", "POST", parsed.path))
                assert payload["okr_id"] == "example-okr"
                write_json_response(
                    self,
                    {"code": 0, "data": {"objective_id": "new-o3", "draft_version": "version-2"}},
                )
                return
            if parsed.path == "/okrx/api/draft_v2/kr/":
                index = 1 + len([event for event in events if event[0] == "create_kr"])
                events.append(("create_kr", "POST", parsed.path))
                assert payload["objective_id"] == "new-o3"
                write_json_response(
                    self,
                    {"code": 0, "data": {"kr_id": f"new-kr-{index}", "draft_version": "version-4"}},
                )
                return
            if parsed.path == "/okrx/api/draft_v2/publish/new-o3/":
                events.append(("publish", "POST", parsed.path))
                assert payload["need_delete_kr_ids"] == []
                assert payload["auto_notify"] is False
                write_json_response(self, {"code": 0, "data": {"draft_version": "version-9"}})
                return
            self.send_error(404)

        def do_PUT(self):
            parsed = urlparse(self.path)
            length = int(self.headers.get("Content-Length", "0"))
            payload = json.loads(self.rfile.read(length).decode("utf-8") or "{}")
            if parsed.path == "/okrx/api/draft_v2/objective/new-o3/":
                events.append(("objective", "PUT", parsed.path))
                assert "Created O3" in payload["name"]
                write_json_response(self, {"code": 0, "data": {"draft_version": "version-3"}})
                return
            if parsed.path in {
                "/okrx/api/draft_v2/kr/new-kr-1/",
                "/okrx/api/draft_v2/kr/new-kr-2/",
            }:
                events.append(("kr_text", "PUT", parsed.path))
                assert "Created KR" in payload["content"]
                write_json_response(self, {"code": 0, "data": {"draft_version": "version-5"}})
                return
            self.send_error(404)

    with serve_handler(Handler) as server:
        result = run_go_ixf(
            binary,
            "okr",
            "write",
            "--url",
            f"{server}/okr/user/example/?okrId=example-okr",
            "--input",
            str(source),
            "--objective-index",
            "3",
            "--cookies",
            str(cookies),
            "--csrf-url",
            f"{server}/lgw/csrf_token",
            "--apply",
        )

    payload = json.loads(result.stdout)
    assert payload["ok"] is True
    assert payload["dryRun"] is False
    assert payload["target"] == {
        "objective": "Created O3",
        "krs": [{"text": "Created KR1"}, {"text": "Created KR2"}],
    }
    assert [event[0] for event in events] == [
        "csrf",
        "detail",
        "version",
        "create_objective",
        "objective",
        "create_kr",
        "kr_text",
        "create_kr",
        "kr_text",
        "publish",
        "detail",
    ]
    assert result.stderr == ""


def test_go_ixf_okr_write_apply_retries_stale_draft_version(tmp_path):
    binary = build_go_ixf(tmp_path)
    source = tmp_path / "okr.json"
    source.write_text(
        json.dumps(
            {
                "objectives": [
                    {
                        "objective": "Retry O3",
                        "krs": ["Retry KR1"],
                    }
                ]
            }
        ),
        encoding="utf-8",
    )
    cookies = tmp_path / "cookies.json"
    write_cookie_fixture(cookies)
    events = []
    objective_versions = []

    def detail_payload(final=False):
        o3 = {
            "id": "o3",
            "name": {"blocks": [{"text": "Retry O3" if final else "Old O3"}]},
            "kr_list": (
                [{"id": "new-kr-1", "content": {"blocks": [{"text": "Retry KR1"}]}}]
                if final
                else [{"id": "old-kr", "content": {"blocks": [{"text": "Old KR"}]}}]
            ),
        }
        return {
            "code": 0,
            "okr_detail_data": {
                "name": "2026 Q3",
                "objective_list": [
                    {"id": "o1", "name": {"blocks": [{"text": "O1"}]}, "kr_list": []},
                    {"id": "o2", "name": {"blocks": [{"text": "O2"}]}, "kr_list": []},
                    o3,
                ],
            },
        }

    class Handler(BaseHTTPRequestHandler):
        def do_GET(self):
            parsed = urlparse(self.path)
            if parsed.path == "/lgw/csrf_token":
                events.append(("csrf", "GET", parsed.path))
                self.send_response(200)
                self.send_header("Set-Cookie", "lgw_csrf_token=lgw-fixture; Path=/")
                self.send_header("Content-Length", "2")
                self.end_headers()
                self.wfile.write(b"{}")
                return
            if parsed.path == "/okrx/api/okr/owner/aggr_detail/":
                events.append(("detail", "GET", parsed.path))
                write_json_response(
                    self,
                    detail_payload(final=any(event[0] == "publish" for event in events)),
                )
                return
            if parsed.path == "/okrx/api/okr/example-okr/version/":
                version_index = 1 + len([event for event in events if event[0] == "version"])
                events.append(("version", "GET", parsed.path))
                write_json_response(
                    self,
                    {"code": 0, "data": {"okr_draft_version": f"version-{version_index}"}},
                )
                return
            self.send_error(404)

        def do_POST(self):
            parsed = urlparse(self.path)
            length = int(self.headers.get("Content-Length", "0"))
            self.rfile.read(length)
            if parsed.path == "/okrx/api/draft_v2/enable/o3/":
                events.append(("enable", "POST", parsed.path))
                write_json_response(self, {"code": 0, "data": {"draft_version": "version-2"}})
                return
            if parsed.path == "/okrx/api/draft_v2/kr/":
                events.append(("create_kr", "POST", parsed.path))
                write_json_response(
                    self,
                    {"code": 0, "data": {"kr_id": "new-kr-1", "draft_version": "version-5"}},
                )
                return
            if parsed.path == "/okrx/api/draft_v2/publish/o3/":
                events.append(("publish", "POST", parsed.path))
                write_json_response(self, {"code": 0, "data": {"draft_version": "version-7"}})
                return
            self.send_error(404)

        def do_PUT(self):
            parsed = urlparse(self.path)
            length = int(self.headers.get("Content-Length", "0"))
            payload = json.loads(self.rfile.read(length).decode("utf-8") or "{}")
            if parsed.path == "/okrx/api/draft_v2/objective/o3/":
                objective_versions.append(payload["draft_version"])
                events.append(("objective", "PUT", parsed.path))
                if len(objective_versions) == 1:
                    write_json_response(self, {"code": 100001, "message": "stale version"})
                    return
                write_json_response(self, {"code": 0, "data": {"draft_version": "version-3"}})
                return
            if parsed.path == "/okrx/api/draft_v2/kr/new-kr-1/":
                events.append(("kr_text", "PUT", parsed.path))
                write_json_response(self, {"code": 0, "data": {"draft_version": "version-6"}})
                return
            self.send_error(404)

        def do_DELETE(self):
            parsed = urlparse(self.path)
            if parsed.path == "/okrx/api/draft_v2/kr/old-kr/":
                events.append(("delete_kr", "DELETE", parsed.path))
                write_json_response(self, {"code": 0, "data": {"draft_version": "version-4"}})
                return
            self.send_error(404)

    with serve_handler(Handler) as server:
        result = run_go_ixf(
            binary,
            "okr",
            "write",
            "--url",
            f"{server}/okr/user/example/?okrId=example-okr",
            "--input",
            str(source),
            "--objective-index",
            "3",
            "--cookies",
            str(cookies),
            "--csrf-url",
            f"{server}/lgw/csrf_token",
            "--apply",
        )

    payload = json.loads(result.stdout)
    assert payload["ok"] is True
    assert payload["target"]["objective"] == "Retry O3"
    assert objective_versions == ["version-2", "version-2"]
    assert [event[0] for event in events] == [
        "csrf",
        "detail",
        "version",
        "enable",
        "objective",
        "version",
        "objective",
        "delete_kr",
        "create_kr",
        "kr_text",
        "publish",
        "detail",
    ]
    assert result.stderr == ""


def test_go_ixf_okr_write_apply_without_index_updates_and_creates_by_objective_text(tmp_path):
    binary = build_go_ixf(tmp_path)
    source = tmp_path / "okr.json"
    source.write_text(
        json.dumps(
            {
                "objectives": [
                    {"objective": "O2", "krs": ["Keep KR", "New KR"]},
                    {"objective": "Created O3", "krs": ["Created KR"]},
                ]
            }
        ),
        encoding="utf-8",
    )
    cookies = tmp_path / "cookies.json"
    write_cookie_fixture(cookies)
    events = []

    def detail_payload(final=False):
        objectives = [
            {"id": "o1", "name": {"blocks": [{"text": "O1"}]}, "kr_list": []},
            {
                "id": "o2",
                "name": {"blocks": [{"text": "O2"}]},
                "kr_list": (
                    [
                        {"id": "keep-kr", "content": {"blocks": [{"text": "Keep KR"}]}},
                        {"id": "new-kr-1", "content": {"blocks": [{"text": "New KR"}]}},
                        {"id": "extra-kr", "content": {"blocks": [{"text": "Extra KR"}]}},
                    ]
                    if final
                    else [
                        {"id": "keep-kr", "content": {"blocks": [{"text": "Keep KR"}]}},
                        {"id": "extra-kr", "content": {"blocks": [{"text": "Extra KR"}]}},
                    ]
                ),
            },
        ]
        if final:
            objectives.append(
                {
                    "id": "new-o3",
                    "name": {"blocks": [{"text": "Created O3"}]},
                    "kr_list": [{"id": "new-kr-2", "content": {"blocks": [{"text": "Created KR"}]}}],
                }
            )
        return {"code": 0, "okr_detail_data": {"name": "2026 Q3", "objective_list": objectives}}

    class Handler(BaseHTTPRequestHandler):
        def do_GET(self):
            parsed = urlparse(self.path)
            if parsed.path == "/lgw/csrf_token":
                events.append(("csrf", "GET", parsed.path))
                self.send_response(200)
                self.send_header("Set-Cookie", "lgw_csrf_token=lgw-fixture; Path=/")
                self.send_header("Content-Length", "2")
                self.end_headers()
                self.wfile.write(b"{}")
                return
            if parsed.path == "/okrx/api/okr/owner/aggr_detail/":
                events.append(("detail", "GET", parsed.path))
                write_json_response(self, detail_payload(final=any(event[0] == "publish_o3" for event in events)))
                return
            if parsed.path == "/okrx/api/okr/example-okr/version/":
                events.append(("version", "GET", parsed.path))
                write_json_response(self, {"code": 0, "data": {"okr_draft_version": "version-1"}})
                return
            self.send_error(404)

        def do_POST(self):
            parsed = urlparse(self.path)
            length = int(self.headers.get("Content-Length", "0"))
            payload = json.loads(self.rfile.read(length).decode("utf-8") or "{}")
            if parsed.path == "/okrx/api/draft_v2/enable/o2/":
                events.append(("enable_o2", "POST", parsed.path))
                write_json_response(self, {"code": 0, "data": {"draft_version": "version-2"}})
                return
            if parsed.path == "/okrx/api/draft_v2/kr/":
                index = 1 + len([event for event in events if event[0] == "create_kr"])
                events.append(("create_kr", "POST", parsed.path))
                assert payload["objective_id"] in {"o2", "new-o3"}
                write_json_response(
                    self,
                    {"code": 0, "data": {"kr_id": f"new-kr-{index}", "draft_version": "version-4"}},
                )
                return
            if parsed.path == "/okrx/api/draft_v2/kr/pos/":
                events.append(("order_krs", "POST", parsed.path))
                assert payload["objectiveId"] == "o2"
                assert payload["krIds"] == ["keep-kr", "new-kr-1", "extra-kr"]
                write_json_response(self, {"code": 0, "data": {"draft_version": "version-5"}})
                return
            if parsed.path == "/okrx/api/draft_v2/publish/o2/":
                events.append(("publish_o2", "POST", parsed.path))
                assert payload["need_delete_kr_ids"] == []
                write_json_response(self, {"code": 0, "data": {"draft_version": "version-6"}})
                return
            if parsed.path == "/okrx/api/draft_v2/objective/":
                events.append(("create_objective", "POST", parsed.path))
                assert payload["okr_id"] == "example-okr"
                write_json_response(
                    self,
                    {"code": 0, "data": {"objective_id": "new-o3", "draft_version": "version-7"}},
                )
                return
            if parsed.path == "/okrx/api/draft_v2/publish/new-o3/":
                events.append(("publish_o3", "POST", parsed.path))
                assert payload["need_delete_kr_ids"] == []
                write_json_response(self, {"code": 0, "data": {"draft_version": "version-10"}})
                return
            self.send_error(404)

        def do_PUT(self):
            parsed = urlparse(self.path)
            length = int(self.headers.get("Content-Length", "0"))
            payload = json.loads(self.rfile.read(length).decode("utf-8") or "{}")
            if parsed.path == "/okrx/api/draft_v2/objective/new-o3/":
                events.append(("objective_o3", "PUT", parsed.path))
                assert "Created O3" in payload["name"]
                write_json_response(self, {"code": 0, "data": {"draft_version": "version-8"}})
                return
            if parsed.path in {"/okrx/api/draft_v2/kr/new-kr-1/", "/okrx/api/draft_v2/kr/new-kr-2/"}:
                events.append(("kr_text", "PUT", parsed.path))
                assert "New KR" in payload["content"] or "Created KR" in payload["content"]
                write_json_response(self, {"code": 0, "data": {"draft_version": "version-9"}})
                return
            self.send_error(404)

        def do_DELETE(self):
            events.append(("unexpected_delete", "DELETE", urlparse(self.path).path))
            self.send_error(404)

    with serve_handler(Handler) as server:
        result = run_go_ixf(
            binary,
            "okr",
            "write",
            "--url",
            f"{server}/okr/user/example/?okrId=example-okr",
            "--input",
            str(source),
            "--cookies",
            str(cookies),
            "--csrf-url",
            f"{server}/lgw/csrf_token",
            "--apply",
        )

    payload = json.loads(result.stdout)
    assert payload["ok"] is True
    assert payload["dryRun"] is False
    assert payload["objectives"] == [
        {"objective": "O1", "krs": []},
        {"objective": "O2", "krs": ["Keep KR", "New KR", "Extra KR"]},
        {"objective": "Created O3", "krs": ["Created KR"]},
    ]
    assert "unexpected_delete" not in [event[0] for event in events]
    assert result.stderr == ""


def test_go_ixf_okr_write_apply_prune_deletes_non_input_content(tmp_path):
    binary = build_go_ixf(tmp_path)
    source = tmp_path / "okr.json"
    source.write_text(
        json.dumps({"objectives": [{"objective": "O2", "krs": ["Pruned KR"]}]}),
        encoding="utf-8",
    )
    cookies = tmp_path / "cookies.json"
    write_cookie_fixture(cookies)
    events = []

    def detail_payload(final=False):
        objectives = [
            {"id": "o1", "name": {"blocks": [{"text": "Delete Me"}]}, "kr_list": []},
            {
                "id": "o2",
                "name": {"blocks": [{"text": "O2"}]},
                "kr_list": (
                    [{"id": "new-kr-1", "content": {"blocks": [{"text": "Pruned KR"}]}}]
                    if final
                    else [
                        {"id": "old-kr", "content": {"blocks": [{"text": "Old KR"}]}},
                        {"id": "extra-kr", "content": {"blocks": [{"text": "Extra KR"}]}},
                    ]
                ),
            },
        ]
        if final:
            objectives = objectives[1:]
        return {"code": 0, "okr_detail_data": {"name": "2026 Q3", "objective_list": objectives}}

    class Handler(BaseHTTPRequestHandler):
        def do_GET(self):
            parsed = urlparse(self.path)
            if parsed.path == "/lgw/csrf_token":
                events.append(("csrf", "GET", parsed.path))
                self.send_response(200)
                self.send_header("Set-Cookie", "lgw_csrf_token=lgw-fixture; Path=/")
                self.send_header("Content-Length", "2")
                self.end_headers()
                self.wfile.write(b"{}")
                return
            if parsed.path == "/okrx/api/okr/owner/aggr_detail/":
                events.append(("detail", "GET", parsed.path))
                write_json_response(self, detail_payload(final=any(event[0] == "publish" for event in events)))
                return
            if parsed.path == "/okrx/api/okr/example-okr/version/":
                events.append(("version", "GET", parsed.path))
                write_json_response(self, {"code": 0, "data": {"okr_draft_version": "version-1"}})
                return
            self.send_error(404)

        def do_POST(self):
            parsed = urlparse(self.path)
            length = int(self.headers.get("Content-Length", "0"))
            payload = json.loads(self.rfile.read(length).decode("utf-8") or "{}")
            if parsed.path == "/okrx/api/draft_v2/enable/o1/":
                events.append(("enable_o1", "POST", parsed.path))
                write_json_response(self, {"code": 0, "data": {"draft_version": "version-2"}})
                return
            if parsed.path == "/okrx/api/draft_v2/enable/o2/":
                events.append(("enable_o2", "POST", parsed.path))
                write_json_response(self, {"code": 0, "data": {"draft_version": "version-3"}})
                return
            if parsed.path == "/okrx/api/draft_v2/kr/":
                events.append(("create_kr", "POST", parsed.path))
                assert payload["objective_id"] == "o2"
                write_json_response(self, {"code": 0, "data": {"kr_id": "new-kr-1", "draft_version": "version-6"}})
                return
            if parsed.path == "/okrx/api/draft_v2/publish/o2/":
                events.append(("publish", "POST", parsed.path))
                assert payload["need_delete_kr_ids"] == ["old-kr", "extra-kr"]
                write_json_response(self, {"code": 0, "data": {"draft_version": "version-8"}})
                return
            self.send_error(404)

        def do_PUT(self):
            parsed = urlparse(self.path)
            length = int(self.headers.get("Content-Length", "0"))
            payload = json.loads(self.rfile.read(length).decode("utf-8") or "{}")
            if parsed.path == "/okrx/api/draft_v2/kr/new-kr-1/":
                events.append(("kr_text", "PUT", parsed.path))
                assert "Pruned KR" in payload["content"]
                write_json_response(self, {"code": 0, "data": {"draft_version": "version-7"}})
                return
            self.send_error(404)

        def do_DELETE(self):
            parsed = urlparse(self.path)
            query = parse_qs(parsed.query)
            assert query["draft_version"]
            if parsed.path == "/okrx/api/draft_v2/objective/o1/":
                events.append(("delete_o1", "DELETE", parsed.path))
                write_json_response(self, {"code": 0, "data": {"draft_version": "version-4"}})
                return
            if parsed.path in {"/okrx/api/draft_v2/kr/old-kr/", "/okrx/api/draft_v2/kr/extra-kr/"}:
                events.append(("delete_kr", "DELETE", parsed.path))
                write_json_response(self, {"code": 0, "data": {"draft_version": "version-5"}})
                return
            self.send_error(404)

    with serve_handler(Handler) as server:
        result = run_go_ixf(
            binary,
            "okr",
            "write",
            "--url",
            f"{server}/okr/user/example/?okrId=example-okr",
            "--input",
            str(source),
            "--cookies",
            str(cookies),
            "--csrf-url",
            f"{server}/lgw/csrf_token",
            "--prune",
            "--apply",
        )

    payload = json.loads(result.stdout)
    assert payload["ok"] is True
    assert payload["dryRun"] is False
    assert payload["objectives"] == [{"objective": "O2", "krs": ["Pruned KR"]}]
    assert [event[0] for event in events if event[0].startswith("delete")] == [
        "delete_o1",
        "delete_kr",
        "delete_kr",
    ]
    assert result.stderr == ""


def test_go_ixf_okr_write_apply_rejects_multi_objective_index_before_cookies(tmp_path):
    binary = build_go_ixf(tmp_path)
    source = tmp_path / "okr.json"
    source.write_text(
        json.dumps(
            {
                "objectives": [
                    {"objective": "New O1", "krs": ["New KR1"]},
                    {"objective": "New O2", "krs": ["New KR2"]},
                ]
            }
        ),
        encoding="utf-8",
    )
    missing_cookies = tmp_path / "missing-cookies.json"

    result = run_go_ixf(
        binary,
        "okr",
        "write",
        "--url",
        "https://tenant.example.test/okr/user/example/?okrId=okr-fixture-200",
        "--input",
        str(source),
        "--objective-index",
        "3",
        "--cookies",
        str(missing_cookies),
        "--apply",
        check=False,
    )

    assert result.returncode == 2
    assert "--objective-index requires exactly one Objective" in result.stderr
    assert str(missing_cookies) not in result.stderr
    assert result.stdout == ""


def test_go_ixf_docs_cleanup_removes_only_manifest_outputs(tmp_path):
    binary = build_go_ixf(tmp_path)
    out_dir = tmp_path / "out"
    asset = out_dir / "assets" / "docx_1" / "image-001.png"
    generated = out_dir / "docx-1.md"
    keep = out_dir / "keep.txt"
    asset.parent.mkdir(parents=True)
    asset.write_bytes(b"image")
    generated.write_text("# Doc\n", encoding="utf-8")
    keep.write_text("keep", encoding="utf-8")
    (out_dir / "manifest.json").write_text(
        json.dumps(
            {
                "docx_1": {
                    "file": str(generated),
                    "assets": [{"path": "assets/docx_1/image-001.png"}],
                }
            }
        ),
        encoding="utf-8",
    )

    run_go_ixf(binary, "docs", "cleanup", str(out_dir))

    assert not generated.exists()
    assert not asset.exists()
    assert not (out_dir / "manifest.json").exists()
    assert keep.read_text(encoding="utf-8") == "keep"


def test_go_ixf_update_self_json_defaults_to_dry_run_with_fixture(tmp_path):
    binary = build_go_ixf(tmp_path)
    release = tmp_path / "latest.json"
    release.write_text(
        json.dumps(
            {
                "tag_name": "v2.7.0",
                "html_url": "https://github.example/releases/v2.7.0",
            }
        ),
        encoding="utf-8",
    )

    result = run_go_ixf(
        binary,
        "update",
        "self",
        "--release-file",
        str(release),
        "--json",
    )
    payload = json.loads(result.stdout)

    assert payload["ok"] is True
    assert payload["currentVersion"] == "2.6.0"
    assert payload["latestVersion"] == "2.7.0"
    assert payload["latestTag"] == "v2.7.0"
    assert payload["updateAvailable"] is True
    assert payload["applied"] is False
    assert payload["commands"] == []
    assert "github.com" in payload["installCommand"]


def test_go_ixf_update_self_apply_replaces_target_with_verified_asset(tmp_path):
    binary = build_go_ixf(tmp_path)
    version = "2.7.0"
    goos = subprocess.run(
        ["go", "env", "GOOS"],
        cwd=ROOT,
        env=GO_ENV,
        text=True,
        capture_output=True,
        check=True,
    ).stdout.strip()
    goarch = subprocess.run(
        ["go", "env", "GOARCH"],
        cwd=ROOT,
        env=GO_ENV,
        text=True,
        capture_output=True,
        check=True,
    ).stdout.strip()
    artifact_name = f"ixf_{version}_{goos}_{goarch}" + (".exe" if os.name == "nt" else "")
    artifact = tmp_path / artifact_name
    replacement = b"new-go-binary\n"
    artifact.write_bytes(replacement)
    checksum = hashlib.sha256(replacement).hexdigest()
    checksums = tmp_path / f"ixf_{version}_checksums.txt"
    checksums.write_text(f"{checksum}  {artifact_name}\n", encoding="utf-8")
    release = tmp_path / "latest.json"
    release.write_text(
        json.dumps(
            {
                "tag_name": f"v{version}",
                "html_url": f"https://github.example/releases/v{version}",
                "assets": [
                    {"name": artifact_name, "browser_download_url": artifact.as_uri()},
                    {"name": checksums.name, "browser_download_url": checksums.as_uri()},
                ],
            }
        ),
        encoding="utf-8",
    )
    target = tmp_path / ("ixf-target.exe" if os.name == "nt" else "ixf-target")
    target.write_bytes(b"old-go-binary\n")

    result = run_go_ixf(
        binary,
        "update",
        "self",
        "--release-file",
        str(release),
        "--target-path",
        str(target),
        "--apply",
        "--json",
    )
    payload = json.loads(result.stdout)

    assert payload["applied"] is True
    assert payload["checksumVerified"] is True
    assert payload["artifactName"] == artifact_name
    assert target.read_bytes() == replacement
