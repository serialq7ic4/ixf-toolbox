from __future__ import annotations

import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
import subprocess
import threading
from pathlib import Path
from urllib.parse import parse_qs, urlparse


ROOT = Path(__file__).resolve().parents[1]
GO_ENV = {
    **os.environ,
    "GOFLAGS": "",
}


def build_go_ixf(tmp_path: Path) -> Path:
    binary = tmp_path / ("ixf-go.exe" if os.name == "nt" else "ixf-go")
    subprocess.run(
        ["go", "build", "-o", str(binary), "./cmd/ixf"],
        cwd=ROOT,
        env=GO_ENV,
        text=True,
        capture_output=True,
        check=True,
    )
    return binary


def run_go_ixf(
    binary: Path,
    *args: str,
    home: Path | None = None,
    check: bool = True,
) -> subprocess.CompletedProcess[str]:
    env = dict(GO_ENV)
    if home is not None:
        env["HOME"] = str(home)
    return subprocess.run(
        [str(binary), *args],
        cwd=ROOT,
        env=env,
        text=True,
        capture_output=True,
        check=check,
    )


def test_go_ixf_version_matches_python_release(tmp_path):
    binary = build_go_ixf(tmp_path)
    result = run_go_ixf(binary, "--version")

    assert result.stdout.strip() == "ixf 1.2.0"
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
    cookies.write_text(
        json.dumps(
            [
                {"name": "_csrf_token", "value": "dummy-csrf"},
                {"name": "session", "value": "dummy-session"},
            ]
        ),
        encoding="utf-8",
    )

    result = run_go_ixf(binary, "doctor", "--cookies", str(cookies), "--json", home=tmp_path)
    payload = json.loads(result.stdout)
    serialized = json.dumps(payload, ensure_ascii=False)

    assert payload["ok"] is True
    assert payload["version"] == "1.2.0"
    assert payload["runtime"] == "go-poc"
    assert payload["skills"]["codex"]["ok"] is True
    assert payload["cookies"]["hasCsrf"] is True
    assert "dummy-csrf" not in serialized
    assert "dummy-session" not in serialized


def test_go_ixf_cookies_export_has_safe_poc_failure(tmp_path):
    binary = build_go_ixf(tmp_path)
    output = tmp_path / "cookies.json"

    result = run_go_ixf(
        binary,
        "cookies",
        "export",
        "--provider",
        "auto",
        "--output",
        str(output),
        "--json",
        check=False,
    )
    payload = json.loads(result.stdout)

    assert result.returncode == 6
    assert payload["ok"] is False
    assert payload["error"]["type"] == "cookie"
    assert payload["error"]["subtype"] == "cookie_export_unavailable"
    assert payload["error"]["retryable"] is False
    assert not output.exists()


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
    cookies.write_text(
        json.dumps(
            [
                {"name": "_csrf_token", "value": "csrf-fixture"},
                {"name": "session", "value": "session-fixture"},
            ]
        ),
        encoding="utf-8",
    )
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
            body = json.dumps(payload).encode("utf-8")
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def log_message(self, format: str, *args: object) -> None:
            return

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    base_url = f"http://127.0.0.1:{server.server_port}"
    try:
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
    finally:
        server.shutdown()
        thread.join(timeout=5)

    assert result.stdout == "# Remote Doc\n\nFirst\n\nLater\n"
    assert result.stderr == ""
    assert errors == []
    assert requested[-1] == "/space/api/docx/pages/client_vars?id=page_1&open_type=1&mode=4&cursor=next-cursor"


def test_go_ixf_docs_read_remote_docx_downloads_images_to_manifest(tmp_path):
    binary = build_go_ixf(tmp_path)
    cookies = tmp_path / "cookies.json"
    cookies.write_text(
        json.dumps(
            [
                {"name": "_csrf_token", "value": "csrf-fixture"},
                {"name": "session", "value": "session-fixture"},
            ]
        ),
        encoding="utf-8",
    )
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
                body = json.dumps(payload).encode("utf-8")
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)
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

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    base_url = f"http://127.0.0.1:{server.server_port}"
    out_dir = tmp_path / "out"
    try:
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
    finally:
        server.shutdown()
        thread.join(timeout=5)

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
                "tag_name": "v1.3.0",
                "html_url": "https://github.example/releases/v1.3.0",
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
    assert payload["currentVersion"] == "1.2.0"
    assert payload["latestVersion"] == "1.3.0"
    assert payload["latestTag"] == "v1.3.0"
    assert payload["updateAvailable"] is True
    assert payload["applied"] is False
    assert payload["commands"] == []
    assert "github.com" in payload["installCommand"]


def test_go_ixf_update_self_apply_replaces_target_with_verified_asset(tmp_path):
    binary = build_go_ixf(tmp_path)
    version = "1.3.0"
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
                "html_url": "https://github.example/releases/v1.3.0",
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
