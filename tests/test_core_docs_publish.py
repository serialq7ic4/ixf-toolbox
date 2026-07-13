import importlib
from pathlib import Path

import pytest


FIXTURE = Path(__file__).parent / "fixtures" / "docs_publish_sample.md"


@pytest.fixture
def publisher():
    try:
        return importlib.import_module("ixf_toolbox.core.docs.publisher")
    except ModuleNotFoundError:
        class MissingPublisher:
            def __getattr__(self, name):
                pytest.fail("missing native docs publisher module")

        return MissingPublisher()


def test_parse_markdown_builds_supported_block_types(publisher):
    title, specs = publisher.parse_markdown(FIXTURE.read_text(encoding="utf-8"))

    assert title == "Writer Sample"
    assert [spec[0] for spec in specs] == [
        "text",
        "heading2",
        "bullet",
        "ordered",
        "code",
        "callout",
        "quote",
        "callout",
    ]
    code = next(spec for spec in specs if spec[0] == "code")
    assert code[1] == "echo one\necho two"


def test_markdown_requires_level_one_title(publisher):
    with pytest.raises(ValueError, match="level-1 title"):
        publisher.parse_markdown("paragraph only")


def test_multiline_attrib_preserves_line_count(publisher):
    assert publisher.attrib_for("one\ntwo") == "*0|1+4*0+3"


def test_build_blocks_keeps_multiline_code_as_one_block(publisher):
    _, specs = publisher.parse_markdown(FIXTURE.read_text(encoding="utf-8"))
    top_ids, entries = publisher.build_blocks(specs, "page-1", publisher.BlockFactory("author-1"))
    blocks = dict(entries)
    code_ids = [block_id for block_id in top_ids if blocks[block_id]["type"] == "code"]

    assert len(code_ids) == 1
    code_text = blocks[code_ids[0]]["text"]["initialAttributedTexts"]["text"]["0"]
    assert code_text == "echo one\necho two"


def test_member_id_defaults_to_authenticated_root_author(publisher):
    root = {"data": {"author": "author-1"}}

    assert publisher.resolve_member_id("", root) == "author-1"
    assert publisher.resolve_member_id("override-1", root) == "override-1"


def test_publish_markdown_dry_run_does_not_require_cookie_file(publisher, tmp_path):
    markdown = tmp_path / "note.md"
    markdown.write_text("# Dry Run\n\nBody.", encoding="utf-8")

    result = publisher.publish_markdown(
        publisher.DocxPublishConfig(
            markdown_path=markdown,
            base_url="https://tenant.example.test",
            apply=False,
        )
    )

    assert result["ok"] is True
    assert result["dryRun"] is True
    assert result["title"] == "Dry Run"
    assert result["counts"] == {"text": 1}
