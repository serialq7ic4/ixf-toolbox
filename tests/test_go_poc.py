from __future__ import annotations

import json
import os
import subprocess
from pathlib import Path


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
