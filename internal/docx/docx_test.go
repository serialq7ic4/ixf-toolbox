package docx

import "testing"

func TestConvertClientVarsRendersBasicMarkdown(t *testing.T) {
	clientVars := map[string]any{
		"block_map": map[string]any{
			"page_1": map[string]any{
				"data": map[string]any{
					"type":     "page",
					"children": []any{"heading_1", "text_1", "bullet_1", "code_1"},
					"text": map[string]any{
						"initialAttributedTexts": map[string]any{
							"text": map[string]any{"0": "Demo Doc"},
						},
					},
				},
			},
			"heading_1": map[string]any{
				"data": map[string]any{
					"type":      "heading1",
					"parent_id": "page_1",
					"text": map[string]any{
						"initialAttributedTexts": map[string]any{
							"text": map[string]any{"0": "Overview"},
						},
					},
				},
			},
			"text_1": map[string]any{
				"data": map[string]any{
					"type":      "text",
					"parent_id": "page_1",
					"text": map[string]any{
						"initialAttributedTexts": map[string]any{
							"text": map[string]any{"0": "Hello world"},
						},
					},
				},
			},
			"bullet_1": map[string]any{
				"data": map[string]any{
					"type":      "bullet",
					"parent_id": "page_1",
					"text": map[string]any{
						"initialAttributedTexts": map[string]any{
							"text": map[string]any{"0": "First point"},
						},
					},
				},
			},
			"code_1": map[string]any{
				"data": map[string]any{
					"type":      "code",
					"parent_id": "page_1",
					"text": map[string]any{
						"initialAttributedTexts": map[string]any{
							"text": map[string]any{"0": "print('hi')"},
						},
					},
				},
			},
		},
	}

	result := ConvertClientVars(clientVars, "page_1")

	want := "# Demo Doc\n\n# Overview\n\nHello world\n\n- First point\n\n```\nprint('hi')\n```\n"
	if result.Markdown != want {
		t.Fatalf("markdown = %q, want %q", result.Markdown, want)
	}
	assertCounts(t, result.Counts, map[string]int{
		"page":     1,
		"heading1": 1,
		"text":     1,
		"bullet":   1,
		"code":     1,
	})
	if len(result.Assets) != 0 {
		t.Fatalf("assets = %#v, want empty", result.Assets)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("warnings = %#v, want empty", result.Warnings)
	}
}

func TestConvertClientVarsRendersRichTextLinks(t *testing.T) {
	clientVars := map[string]any{
		"block_map": map[string]any{
			"page_1": blockData(map[string]any{
				"type":     "page",
				"children": []any{"text_1"},
			}),
			"text_1": blockData(map[string]any{
				"type":      "text",
				"parent_id": "page_1",
				"text": map[string]any{
					"apool": map[string]any{
						"numToAttrib": map[string]any{
							"0": []any{[]any{"url", "https://example.com/spec"}},
						},
					},
					"initialAttributedTexts": map[string]any{
						"attribs": map[string]any{"0": "*0+4", "1": "+6"},
						"text":    map[string]any{"0": "Spec", "1": " ready"},
					},
				},
			}),
		},
	}

	result := ConvertClientVars(clientVars, "page_1")

	if result.Markdown != "[Spec](https://example.com/spec) ready\n" {
		t.Fatalf("markdown = %q", result.Markdown)
	}
	assertCounts(t, result.Counts, map[string]int{"page": 1, "text": 1})
	assertStringSlice(t, result.Warnings, nil)
}

func TestConvertClientVarsRendersTodosTablesAndResourceMarkers(t *testing.T) {
	clientVars := map[string]any{
		"block_map": map[string]any{
			"page_1": blockData(map[string]any{
				"type":     "page",
				"children": []any{"todo_1", "todo_2", "table_1", "image_1", "mindnote_1"},
			}),
			"todo_1": blockData(map[string]any{
				"type":      "todo",
				"parent_id": "page_1",
				"checked":   false,
				"text":      attributedText("Open task"),
			}),
			"todo_2": blockData(map[string]any{
				"type":      "todo",
				"parent_id": "page_1",
				"checked":   true,
				"text":      attributedText("Done task"),
			}),
			"table_1": blockData(map[string]any{
				"type":       "table",
				"parent_id":  "page_1",
				"rows_id":    []any{"row_1", "row_2"},
				"columns_id": []any{"col_1", "col_2"},
				"cell_set": map[string]any{
					"row_1_col_1": map[string]any{"block_id": "cell_1_1"},
					"row_1_col_2": map[string]any{"block_id": "cell_1_2"},
					"row_2_col_1": map[string]any{"block_id": "cell_2_1"},
					"row_2_col_2": map[string]any{"block_id": "cell_2_2"},
				},
			}),
			"cell_1_1": blockData(map[string]any{"type": "table_cell", "children": []any{"text_1_1"}}),
			"cell_1_2": blockData(map[string]any{"type": "table_cell", "children": []any{"text_1_2"}}),
			"cell_2_1": blockData(map[string]any{"type": "table_cell", "children": []any{"text_2_1"}}),
			"cell_2_2": blockData(map[string]any{"type": "table_cell", "children": []any{"text_2_2"}}),
			"text_1_1": blockData(map[string]any{"type": "text", "parent_id": "cell_1_1", "text": attributedText("Name")}),
			"text_1_2": blockData(map[string]any{"type": "text", "parent_id": "cell_1_2", "text": attributedText("Value")}),
			"text_2_1": blockData(map[string]any{"type": "text", "parent_id": "cell_2_1", "text": attributedText("Alpha")}),
			"text_2_2": blockData(map[string]any{"type": "text", "parent_id": "cell_2_2", "text": attributedText("42")}),
			"image_1":  blockData(map[string]any{"type": "image", "parent_id": "page_1"}),
			"mindnote_1": blockData(map[string]any{
				"type":      "mindnote",
				"parent_id": "page_1",
			}),
		},
	}

	result := ConvertClientVars(clientVars, "page_1")

	want := "- [ ] Open task\n\n- [x] Done task\n\n| Name | Value |\n| --- | --- |\n| Alpha | 42 |\n\n[image]\n\n[mindnote]\n"
	if result.Markdown != want {
		t.Fatalf("markdown = %q, want %q", result.Markdown, want)
	}
	assertCounts(t, result.Counts, map[string]int{
		"page":       1,
		"todo":       2,
		"table":      1,
		"table_cell": 4,
		"text":       4,
		"image":      1,
		"mindnote":   1,
	})
	assertStringSlice(t, result.Warnings, nil)
}

func TestConvertClientVarsPreservesUnknownBlocksAndIndentsChildren(t *testing.T) {
	clientVars := map[string]any{
		"block_map": map[string]any{
			"page_1": blockData(map[string]any{
				"type":     "page",
				"children": []any{"unknown_1"},
			}),
			"unknown_1": blockData(map[string]any{
				"type":      "unsupported_widget",
				"parent_id": "page_1",
				"children":  []any{"bullet_1"},
			}),
			"bullet_1": blockData(map[string]any{
				"type":      "bullet",
				"parent_id": "unknown_1",
				"text":      attributedText("Nested text"),
			}),
		},
	}

	result := ConvertClientVars(clientVars, "page_1")

	if result.Markdown != "  - Nested text\n" {
		t.Fatalf("markdown = %q", result.Markdown)
	}
	assertStringSlice(t, result.Warnings, []string{"unsupported block type: unsupported_widget"})
}

func TestConvertClientVarsRendersCalloutsAndEmptyQuotes(t *testing.T) {
	clientVars := map[string]any{
		"block_map": map[string]any{
			"page_1": blockData(map[string]any{
				"type":     "page",
				"children": []any{"callout_1", "quote_1"},
			}),
			"callout_1": blockData(map[string]any{
				"type":      "callout",
				"parent_id": "page_1",
				"children":  []any{"text_1"},
			}),
			"text_1": blockData(map[string]any{
				"type":      "text",
				"parent_id": "callout_1",
				"text":      attributedText("Important note"),
			}),
			"quote_1": blockData(map[string]any{
				"type":      "quote_container",
				"parent_id": "page_1",
				"children":  []any{},
			}),
		},
	}

	result := ConvertClientVars(clientVars, "page_1")

	if result.Markdown != "[callout]\n\nImportant note\n\n>\n" {
		t.Fatalf("markdown = %q", result.Markdown)
	}
	assertStringSlice(t, result.Warnings, nil)
}

func assertCounts(t *testing.T, got map[string]int, want map[string]int) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("counts = %#v, want %#v", got, want)
	}
	for key, wantValue := range want {
		if got[key] != wantValue {
			t.Fatalf("counts[%q] = %d, want %d; all counts %#v", key, got[key], wantValue, got)
		}
	}
}

func blockData(data map[string]any) map[string]any {
	return map[string]any{"data": data}
}

func attributedText(text string) map[string]any {
	return map[string]any{
		"initialAttributedTexts": map[string]any{
			"text": map[string]any{"0": text},
		},
	}
}

func assertStringSlice(t *testing.T, got []string, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("slice = %#v, want %#v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("slice[%d] = %q, want %q; all values %#v", index, got[index], want[index], got)
		}
	}
}
