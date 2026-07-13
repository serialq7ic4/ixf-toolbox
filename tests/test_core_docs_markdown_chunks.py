from ixf_toolbox.core.docs.markdown_chunks import build_outline, render_chunk


def test_outline_uses_h2_below_single_title_h1() -> None:
    markdown = "# Title\n\n## One\n\nAlpha\n\n## Two\n\nBeta\n"

    outline = build_outline(markdown, target_chars=100)

    assert outline.selected_heading_level == 2
    assert [chunk.breadcrumb for chunk in outline.chunks][-2:] == [
        "Title > One",
        "Title > Two",
    ]


def test_outline_uses_h1_for_multiple_substantive_h1_sections() -> None:
    markdown = "# First\n\nAlpha\n\n# Second\n\nBeta\n"

    outline = build_outline(markdown, target_chars=100)

    assert outline.selected_heading_level == 1
    assert [chunk.breadcrumb for chunk in outline.chunks] == ["First", "Second"]


def test_outline_ignores_headings_inside_fenced_code() -> None:
    markdown = "# Title\n\n## Real\n\n```python\n# fake heading\nprint('x')\n```\n"

    outline = build_outline(markdown, target_chars=20)

    assert outline.selected_heading_level == 2
    assert any("# fake heading" in render_chunk(markdown, outline, chunk.index) for chunk in outline.chunks)
    code_chunks = [
        render_chunk(markdown, outline, chunk.index)
        for chunk in outline.chunks
        if "```python" in render_chunk(markdown, outline, chunk.index)
    ]
    assert code_chunks == ["```python\n# fake heading\nprint('x')\n```\n"]


def test_oversized_h2_section_splits_at_h3_with_breadcrumbs() -> None:
    markdown = (
        "# Title\n\n## Long\n\n"
        "### Part A\n\n" + "A" * 30 + "\n\n"
        "### Part B\n\n" + "B" * 30 + "\n"
    )

    outline = build_outline(markdown, target_chars=35)

    assert [chunk.breadcrumb for chunk in outline.chunks if "Part" in chunk.breadcrumb] == [
        "Title > Long > Part A",
        "Title > Long > Part B",
    ]


def test_tables_and_images_remain_atomic_and_images_are_indexed() -> None:
    markdown = (
        "# Title\n\n## Data\n\n"
        "| A | B |\n| --- | --- |\n| 1 | 2 |\n\n"
        "![Diagram](assets/docx_1/image-001.png)\n*Architecture caption*\n"
    )

    outline = build_outline(markdown, target_chars=20)
    rendered = [render_chunk(markdown, outline, chunk.index) for chunk in outline.chunks]

    assert any("| A | B |\n| --- | --- |\n| 1 | 2 |" in chunk for chunk in rendered)
    assert any(
        "![Diagram](assets/docx_1/image-001.png)\n*Architecture caption*" in chunk
        for chunk in rendered
    )
    assert [path for chunk in outline.chunks for path in chunk.image_paths] == [
        "assets/docx_1/image-001.png"
    ]
