import json
from collections import Counter
from dataclasses import FrozenInstanceError

import pytest

from ixf_toolbox.core.docs.converters.docx_markdown import (
    ConversionOptions,
    ImageReference,
    ImageResolution,
    convert_docx_client_vars,
)


def test_convert_docx_client_vars_renders_basic_markdown() -> None:
    client_vars = {
        "block_map": {
            "page_1": {
                "data": {
                    "type": "page",
                    "children": ["heading_1", "text_1", "bullet_1", "code_1"],
                    "text": {"initialAttributedTexts": {"text": {"0": "Demo Doc"}}},
                }
            },
            "heading_1": {
                "data": {
                    "type": "heading1",
                    "parent_id": "page_1",
                    "text": {"initialAttributedTexts": {"text": {"0": "Overview"}}},
                }
            },
            "text_1": {
                "data": {
                    "type": "text",
                    "parent_id": "page_1",
                    "text": {"initialAttributedTexts": {"text": {"0": "Hello world"}}},
                }
            },
            "bullet_1": {
                "data": {
                    "type": "bullet",
                    "parent_id": "page_1",
                    "text": {"initialAttributedTexts": {"text": {"0": "First point"}}},
                }
            },
            "code_1": {
                "data": {
                    "type": "code",
                    "parent_id": "page_1",
                    "text": {"initialAttributedTexts": {"text": {"0": "print('hi')"}}},
                }
            },
        }
    }

    result = convert_docx_client_vars(client_vars, "page_1")

    assert result.markdown == (
        "# Demo Doc\n\n"
        "# Overview\n\n"
        "Hello world\n\n"
        "- First point\n\n"
        "```\n"
        "print('hi')\n"
        "```\n"
    )
    assert result.counts == Counter({"page": 1, "heading1": 1, "text": 1, "bullet": 1, "code": 1})
    assert result.assets == []
    assert result.warnings == []


def test_convert_docx_client_vars_renders_code_language() -> None:
    client_vars = {
        "block_map": {
            "page_1": {"data": {"type": "page", "children": ["code_1"]}},
            "code_1": {
                "data": {
                    "type": "code",
                    "parent_id": "page_1",
                    "language": "python",
                    "text": {"initialAttributedTexts": {"text": {"0": "print('hi')"}}},
                }
            },
        }
    }

    result = convert_docx_client_vars(client_vars, "page_1")

    assert result.markdown == "```python\nprint('hi')\n```\n"
    assert result.counts == Counter({"page": 1, "code": 1})
    assert result.warnings == []


def test_convert_docx_client_vars_renders_todo_items() -> None:
    client_vars = {
        "block_map": {
            "page_1": {"data": {"type": "page", "children": ["todo_1", "todo_2"]}},
            "todo_1": {
                "data": {
                    "type": "todo",
                    "parent_id": "page_1",
                    "checked": False,
                    "text": {"initialAttributedTexts": {"text": {"0": "Open task"}}},
                }
            },
            "todo_2": {
                "data": {
                    "type": "todo",
                    "parent_id": "page_1",
                    "checked": True,
                    "text": {"initialAttributedTexts": {"text": {"0": "Done task"}}},
                }
            },
        }
    }

    result = convert_docx_client_vars(client_vars, "page_1")

    assert result.markdown == "- [ ] Open task\n\n- [x] Done task\n"
    assert result.counts == Counter({"page": 1, "todo": 2})
    assert result.warnings == []


def test_convert_docx_client_vars_renders_rich_text_links() -> None:
    client_vars = {
        "block_map": {
            "page_1": {"data": {"type": "page", "children": ["text_1"]}},
            "text_1": {
                "data": {
                    "type": "text",
                    "parent_id": "page_1",
                    "text": {
                        "apool": {
                            "numToAttrib": {
                                "0": [["url", "https://example.com/spec"]],
                            }
                        },
                        "initialAttributedTexts": {
                            "attribs": {"0": "*0+4", "1": "+6"},
                            "text": {"0": "Spec", "1": " ready"},
                        },
                    },
                }
            },
        }
    }

    result = convert_docx_client_vars(client_vars, "page_1")

    assert result.markdown == "[Spec](https://example.com/spec) ready\n"
    assert result.counts == Counter({"page": 1, "text": 1})
    assert result.warnings == []


def test_convert_docx_client_vars_marks_unknown_blocks_without_losing_children() -> None:
    client_vars = {
        "block_map": {
            "page_1": {"data": {"type": "page", "children": ["unknown_1"]}},
            "unknown_1": {
                "data": {
                    "type": "unsupported_widget",
                    "parent_id": "page_1",
                    "children": ["text_1"],
                }
            },
            "text_1": {
                "data": {
                    "type": "text",
                    "parent_id": "unknown_1",
                    "text": {"initialAttributedTexts": {"text": {"0": "Nested text"}}},
                }
            },
        }
    }

    result = convert_docx_client_vars(client_vars, "page_1")

    assert result.markdown == "Nested text\n"
    assert result.warnings == ["unsupported block type: unsupported_widget"]


def test_convert_docx_client_vars_expands_sheet_blocks() -> None:
    client_vars = {
        "block_map": {
            "page_1": {"data": {"type": "page", "children": ["sheet_1"]}},
            "sheet_1": {
                "data": {
                    "type": "sheet",
                    "parent_id": "page_1",
                    "token": "shtr_fixture_sheet1",
                }
            },
        }
    }
    expanded_tokens: list[str] = []

    def expand_sheet(token: str) -> list[str]:
        expanded_tokens.append(token)
        return [
            "[sheet-meta workbook_token=shtr_fixture sheet_id=sheet1 rows=1 cols=2]",
            "Name\tValue",
        ]

    result = convert_docx_client_vars(
        client_vars,
        "page_1",
        ConversionOptions(expand_sheet=expand_sheet),
    )

    assert expanded_tokens == ["shtr_fixture_sheet1"]
    assert result.markdown == (
        "[sheet token=shtr_fixture_sheet1]\n"
        "[sheet-meta workbook_token=shtr_fixture sheet_id=sheet1 rows=1 cols=2]\n"
        "Name\tValue\n"
    )
    assert result.counts == Counter({"page": 1, "sheet": 1})
    assert result.warnings == []


def test_convert_docx_client_vars_preserves_resource_markers() -> None:
    client_vars = {
        "block_map": {
            "page_1": {
                "data": {
                    "type": "page",
                    "children": ["table_1", "image_1", "mindnote_1"],
                }
            },
            "table_1": {"data": {"type": "table", "parent_id": "page_1"}},
            "image_1": {"data": {"type": "image", "parent_id": "page_1"}},
            "mindnote_1": {"data": {"type": "mindnote", "parent_id": "page_1"}},
        }
    }

    result = convert_docx_client_vars(client_vars, "page_1")

    assert result.markdown == "[table]\n\n[image]\n\n[mindnote]\n"
    assert result.counts == Counter({"page": 1, "table": 1, "image": 1, "mindnote": 1})
    assert result.warnings == []


def test_convert_docx_client_vars_resolves_image_metadata_and_renders_markdown() -> None:
    token = "raw-image-token"
    client_vars = {
        "block_map": {
            "page_1": {"data": {"type": "page", "children": ["image_1"]}},
            "image_1": {
                "data": {
                    "type": "image",
                    "parent_id": "page_1",
                    "image": {
                        "token": token,
                        "name": "architecture.png",
                        "mimeType": "image/png",
                        "width": 1200,
                        "height": 800,
                        "size": 1234,
                        "caption": {
                            "initialAttributedTexts": {
                                "text": {"0": "Architecture diagram"},
                            }
                        },
                    },
                }
            },
        }
    }
    resolution = ImageResolution(
        markdown_path="assets/docx_1/image-001.png",
        alt_text="Architecture diagram",
        asset={
            "path": "assets/docx_1/image-001.png",
            "mimeType": "image/png",
            "width": 1200,
            "height": 800,
            "sizeBytes": 1234,
            "status": "downloaded",
            "ordinal": 1,
        },
    )
    received_references: list[ImageReference] = []

    def resolve_image(
        reference: ImageReference,
    ) -> ImageResolution:
        received_references.append(reference)
        return resolution

    result = convert_docx_client_vars(
        client_vars,
        "page_1",
        ConversionOptions(resolve_image=resolve_image),
    )

    assert received_references == [
        ImageReference(
            block_id="image_1",
            token=token,
            name="architecture.png",
            mime_type="image/png",
            width=1200,
            height=800,
            declared_size=1234,
            caption="Architecture diagram",
        )
    ]
    assert result.markdown == "![Architecture diagram](assets/docx_1/image-001.png)\n"
    assert result.assets == [resolution.asset]
    assert result.warnings == []
    assert token not in result.markdown
    assert token not in json.dumps(result.assets, sort_keys=True)
    with pytest.raises(FrozenInstanceError):
        received_references[0].token = "changed"
    with pytest.raises(FrozenInstanceError):
        resolution.alt_text = "changed"


def test_convert_docx_client_vars_preserves_image_marker_and_collects_warning_without_path() -> None:
    token = "raw-image-token"
    client_vars = {
        "block_map": {
            "page_1": {"data": {"type": "page", "children": ["image_1"]}},
            "image_1": {
                "data": {
                    "type": "image",
                    "parent_id": "page_1",
                    "image": {"token": token},
                    "caption": {
                        "initialAttributedTexts": {
                            "text": {"0": "Architecture diagram"},
                        }
                    },
                }
            },
        }
    }

    result = convert_docx_client_vars(
        client_vars,
        "page_1",
        ConversionOptions(
            resolve_image=lambda _reference: ImageResolution(
                markdown_path=None,
                alt_text="Architecture diagram",
                warning="image 1 download failed: http_error",
            )
        ),
    )

    assert result.markdown == "[image]\n"
    assert result.assets == []
    assert result.warnings == ["image 1 download failed: http_error"]
    assert token not in result.markdown
    assert token not in json.dumps(result.warnings)


def test_convert_docx_client_vars_preserves_image_marker_when_resolver_raises() -> None:
    token = "raw-image-token"
    client_vars = {
        "block_map": {
            "page_1": {"data": {"type": "page", "children": ["image_1"]}},
            "image_1": {
                "data": {
                    "type": "image",
                    "parent_id": "page_1",
                    "image": {"token": token},
                }
            },
        }
    }

    def resolve_image(_reference: ImageReference) -> ImageResolution:
        raise RuntimeError(f"download failed for {token}: private resolver detail")

    result = convert_docx_client_vars(
        client_vars,
        "page_1",
        ConversionOptions(resolve_image=resolve_image),
    )

    assert result.markdown == "[image]\n"
    assert result.assets == []
    assert result.warnings == ["image resolution failed"]
    serialized_warnings = json.dumps(result.warnings)
    assert token not in serialized_warnings
    assert "private resolver detail" not in serialized_warnings


@pytest.mark.parametrize(
    "leak_target",
    ["markdown_path", "alt_text", "asset", "warning"],
)
def test_convert_docx_client_vars_rejects_resolver_output_containing_image_token(
    leak_target: str,
) -> None:
    token = "raw-image-token"
    client_vars = {
        "block_map": {
            "page_1": {"data": {"type": "page", "children": ["image_1"]}},
            "image_1": {
                "data": {
                    "type": "image",
                    "parent_id": "page_1",
                    "image": {"token": token},
                }
            },
        }
    }
    resolution_values = {
        "markdown_path": "assets/docx_1/image-001.png",
        "alt_text": "Architecture diagram",
        "asset": {
            "path": "assets/docx_1/image-001.png",
            "status": "downloaded",
        },
        "warning": None,
    }
    if leak_target == "markdown_path":
        resolution_values["markdown_path"] = f"assets/{token}/image-001.png"
    elif leak_target == "alt_text":
        resolution_values["alt_text"] = f"Architecture diagram {token}"
    elif leak_target == "asset":
        resolution_values["asset"] = {"source": {"resourceToken": token}}
    else:
        resolution_values["warning"] = f"image resolution failed for {token}"

    result = convert_docx_client_vars(
        client_vars,
        "page_1",
        ConversionOptions(
            resolve_image=lambda _reference: ImageResolution(**resolution_values),
        ),
    )

    assert result.markdown == "[image]\n"
    assert result.assets == []
    assert result.warnings == ["image resolution rejected unsafe output"]
    serialized_result = json.dumps(
        {
            "markdown": result.markdown,
            "assets": result.assets,
            "warnings": result.warnings,
        },
        sort_keys=True,
    )
    assert token not in serialized_result


def test_convert_docx_client_vars_renders_simple_tables() -> None:
    client_vars = {
        "block_map": {
            "page_1": {"data": {"type": "page", "children": ["table_1"]}},
            "table_1": {
                "data": {
                    "type": "table",
                    "parent_id": "page_1",
                    "rows_id": ["row_1", "row_2"],
                    "columns_id": ["col_1", "col_2"],
                    "cell_set": {
                        "row_1_col_1": {"block_id": "cell_1_1"},
                        "row_1_col_2": {"block_id": "cell_1_2"},
                        "row_2_col_1": {"block_id": "cell_2_1"},
                        "row_2_col_2": {"block_id": "cell_2_2"},
                    },
                }
            },
            "cell_1_1": {"data": {"type": "table_cell", "children": ["text_1_1"]}},
            "cell_1_2": {"data": {"type": "table_cell", "children": ["text_1_2"]}},
            "cell_2_1": {"data": {"type": "table_cell", "children": ["text_2_1"]}},
            "cell_2_2": {"data": {"type": "table_cell", "children": ["text_2_2"]}},
            "text_1_1": {
                "data": {
                    "type": "text",
                    "parent_id": "cell_1_1",
                    "text": {"initialAttributedTexts": {"text": {"0": "Name"}}},
                }
            },
            "text_1_2": {
                "data": {
                    "type": "text",
                    "parent_id": "cell_1_2",
                    "text": {"initialAttributedTexts": {"text": {"0": "Value"}}},
                }
            },
            "text_2_1": {
                "data": {
                    "type": "text",
                    "parent_id": "cell_2_1",
                    "text": {"initialAttributedTexts": {"text": {"0": "Alpha"}}},
                }
            },
            "text_2_2": {
                "data": {
                    "type": "text",
                    "parent_id": "cell_2_2",
                    "text": {"initialAttributedTexts": {"text": {"0": "42"}}},
                }
            },
        }
    }

    result = convert_docx_client_vars(client_vars, "page_1")

    assert result.markdown == (
        "| Name | Value |\n"
        "| --- | --- |\n"
        "| Alpha | 42 |\n"
    )
    assert result.counts == Counter({"page": 1, "table": 1, "table_cell": 4, "text": 4})
    assert result.warnings == []


def test_convert_docx_client_vars_numbers_ordered_siblings() -> None:
    client_vars = {
        "block_map": {
            "page_1": {
                "data": {
                    "type": "page",
                    "children": ["ordered_1", "ordered_2"],
                }
            },
            "ordered_1": {
                "data": {
                    "type": "ordered",
                    "parent_id": "page_1",
                    "text": {"initialAttributedTexts": {"text": {"0": "First"}}},
                }
            },
            "ordered_2": {
                "data": {
                    "type": "ordered",
                    "parent_id": "page_1",
                    "text": {"initialAttributedTexts": {"text": {"0": "Second"}}},
                }
            },
        }
    }

    result = convert_docx_client_vars(client_vars, "page_1")

    assert result.markdown == "1. First\n\n2. Second\n"
    assert result.counts == Counter({"page": 1, "ordered": 2})
    assert result.warnings == []


def test_convert_docx_client_vars_indents_nested_bullets() -> None:
    client_vars = {
        "block_map": {
            "page_1": {
                "data": {
                    "type": "page",
                    "children": ["bullet_1"],
                }
            },
            "bullet_1": {
                "data": {
                    "type": "bullet",
                    "parent_id": "page_1",
                    "children": ["bullet_2"],
                    "text": {"initialAttributedTexts": {"text": {"0": "Parent"}}},
                }
            },
            "bullet_2": {
                "data": {
                    "type": "bullet",
                    "parent_id": "bullet_1",
                    "text": {"initialAttributedTexts": {"text": {"0": "Child"}}},
                }
            },
        }
    }

    result = convert_docx_client_vars(client_vars, "page_1")

    assert result.markdown == "- Parent\n\n  - Child\n"
    assert result.counts == Counter({"page": 1, "bullet": 2})
    assert result.warnings == []


def test_convert_docx_client_vars_indents_bullets_inside_unknown_containers() -> None:
    client_vars = {
        "block_map": {
            "page_1": {
                "data": {
                    "type": "page",
                    "children": ["container_1"],
                }
            },
            "container_1": {
                "data": {
                    "type": "okr_container",
                    "parent_id": "page_1",
                    "children": ["bullet_1"],
                }
            },
            "bullet_1": {
                "data": {
                    "type": "bullet",
                    "parent_id": "container_1",
                    "text": {"initialAttributedTexts": {"text": {"0": "Nested in container"}}},
                }
            },
        }
    }

    result = convert_docx_client_vars(client_vars, "page_1")

    assert result.markdown == "  - Nested in container\n"
    assert result.counts == Counter({"page": 1, "okr_container": 1, "bullet": 1})
    assert result.warnings == ["unsupported block type: okr_container"]


def test_convert_docx_client_vars_indents_bullets_inside_callouts() -> None:
    client_vars = {
        "block_map": {
            "page_1": {
                "data": {
                    "type": "page",
                    "children": ["callout_1"],
                }
            },
            "callout_1": {
                "data": {
                    "type": "callout",
                    "parent_id": "page_1",
                    "children": ["bullet_1"],
                }
            },
            "bullet_1": {
                "data": {
                    "type": "bullet",
                    "parent_id": "callout_1",
                    "text": {"initialAttributedTexts": {"text": {"0": "Callout child"}}},
                }
            },
        }
    }

    result = convert_docx_client_vars(client_vars, "page_1")

    assert result.markdown == "[callout]\n\n  - Callout child\n"
    assert result.counts == Counter({"page": 1, "callout": 1, "bullet": 1})
    assert result.warnings == []


def test_convert_docx_client_vars_preserves_callout_marker() -> None:
    client_vars = {
        "block_map": {
            "page_1": {"data": {"type": "page", "children": ["callout_1"]}},
            "callout_1": {
                "data": {
                    "type": "callout",
                    "parent_id": "page_1",
                    "children": ["text_1"],
                }
            },
            "text_1": {
                "data": {
                    "type": "text",
                    "parent_id": "callout_1",
                    "text": {"initialAttributedTexts": {"text": {"0": "Important note"}}},
                }
            },
        }
    }

    result = convert_docx_client_vars(client_vars, "page_1")

    assert result.markdown == "[callout]\n\nImportant note\n"
    assert result.counts == Counter({"page": 1, "callout": 1, "text": 1})
    assert result.warnings == []


def test_convert_docx_client_vars_preserves_empty_quote_marker() -> None:
    client_vars = {
        "block_map": {
            "page_1": {"data": {"type": "page", "children": ["quote_1"]}},
            "quote_1": {
                "data": {
                    "type": "quote_container",
                    "parent_id": "page_1",
                    "children": [],
                }
            },
        }
    }

    result = convert_docx_client_vars(client_vars, "page_1")

    assert result.markdown == ">\n"
    assert result.counts == Counter({"page": 1, "quote_container": 1})
    assert result.warnings == []
