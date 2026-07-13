from __future__ import annotations

import json

from ixf_toolbox.core.docs import (
    build_outline,
    cleanup_outputs,
    inspect_source,
    read_sources,
    render_chunk,
    write_outputs,
)


def test_read_sources_reads_local_markdown_without_remote_session(tmp_path):
    source = tmp_path / "source.md"
    source.write_text("# Source\n\nHello from local file.\n", encoding="utf-8")

    results = read_sources([str(source)])

    assert results == [
        {
            "source": str(source),
            "kind": "local_markdown",
            "title": "source.md",
            "token": "",
            "content": "# Source\n\nHello from local file.\n",
            "counts": {},
            "assets": [],
            "warnings": [],
        }
    ]


def test_write_outputs_uses_source_stems_and_manifest(tmp_path):
    out_dir = tmp_path / "out"
    source_a = tmp_path / "Project Plan.md"
    source_b = tmp_path / "project-plan.md"
    source_a.write_text("# A\n", encoding="utf-8")
    source_b.write_text("# B\n", encoding="utf-8")
    results = read_sources([str(source_a), str(source_b)])

    manifest = write_outputs(results, out_dir)

    assert manifest["local_markdown_1"]["file"] == str(out_dir / "project-plan.md")
    assert manifest["local_markdown_2"]["file"] == str(out_dir / "project-plan-2.md")
    assert (out_dir / "project-plan.md").read_text(encoding="utf-8") == "# A\n"
    assert (out_dir / "manifest.json").read_text(encoding="utf-8") == json.dumps(
        manifest,
        ensure_ascii=False,
        indent=2,
    )


def test_cleanup_outputs_removes_only_generated_paths(tmp_path):
    out_dir = tmp_path / "out"
    asset = out_dir / "assets" / "docx_1" / "image-001.png"
    markdown = out_dir / "docx-1.md"
    keep = out_dir / "keep.txt"
    asset.parent.mkdir(parents=True)
    asset.write_bytes(b"image")
    markdown.write_text("# Doc\n", encoding="utf-8")
    keep.write_text("keep", encoding="utf-8")
    manifest = {
        "docx_1": {
            "file": str(markdown),
            "assets": [{"path": "assets/docx_1/image-001.png"}],
        }
    }
    (out_dir / "manifest.json").write_text("{}", encoding="utf-8")

    cleanup_outputs(manifest, out_dir)

    assert not markdown.exists()
    assert not asset.exists()
    assert keep.read_text(encoding="utf-8") == "keep"
    assert not (out_dir / "manifest.json").exists()


def test_inspect_source_redacts_remote_tokens_and_preserves_local_metadata(tmp_path):
    source = tmp_path / "private-source.md"
    source.write_text("# Secret Title\n\nSensitive body should not appear.\n", encoding="utf-8")

    local = inspect_source(str(source))
    remote = inspect_source("https://tenant.example.test/docx/doxfixturetoken?from=copy")

    assert local["kind"] == "local_markdown"
    assert local["sizeBytes"] == source.stat().st_size
    assert "Secret Title" not in json.dumps(local, ensure_ascii=False)
    assert remote == {
        "ok": True,
        "sourceRef": "https://tenant.example.test/docx/<redacted>?from=copy",
        "remote": True,
        "kind": "docx",
        "host": "tenant.example.test",
        "pathType": "docx",
        "tokenPrefix": "dox",
        "tokenLength": len("doxfixturetoken"),
        "route": "docx_client_vars",
    }


def test_outline_and_chunk_keep_code_blocks_atomic():
    markdown = "# Title\n\n## Real\n\n```python\n# fake heading\nprint('x')\n```\n"

    outline = build_outline(markdown, target_chars=20)
    rendered_chunks = [render_chunk(markdown, outline, chunk.index) for chunk in outline.chunks]

    assert outline.selected_heading_level == 2
    assert "```python\n# fake heading\nprint('x')\n```\n" in rendered_chunks
