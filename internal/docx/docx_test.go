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

func TestConvertClientVarsExpandsSheetBlocks(t *testing.T) {
	clientVars := map[string]any{
		"block_map": map[string]any{
			"page_1": blockData(map[string]any{
				"type":     "page",
				"children": []any{"sheet_1"},
			}),
			"sheet_1": blockData(map[string]any{
				"type":      "sheet",
				"parent_id": "page_1",
				"token":     "shtr_fixture_sheet1",
			}),
		},
	}
	expandedTokens := []string{}

	result := ConvertClientVarsWithOptions(clientVars, "page_1", Options{
		ExpandSheet: func(token string) []string {
			expandedTokens = append(expandedTokens, token)
			return []string{
				"[sheet-meta workbook_token=shtr_fixture sheet_id=sheet1 rows=1 cols=2]",
				"```tsv",
				"Name\tValue",
				"```",
			}
		},
	})

	assertStringSlice(t, expandedTokens, []string{"shtr_fixture_sheet1"})
	if result.Markdown != "[sheet token=shtr_fixture_sheet1]\n[sheet-meta workbook_token=shtr_fixture sheet_id=sheet1 rows=1 cols=2]\n```tsv\nName\tValue\n```\n" {
		t.Fatalf("markdown = %q", result.Markdown)
	}
	assertCounts(t, result.Counts, map[string]int{"page": 1, "sheet": 1})
	assertStringSlice(t, result.Warnings, nil)
}

func TestConvertClientVarsResolvesImageMetadataAndRendersMarkdown(t *testing.T) {
	token := "raw-image-token"
	clientVars := map[string]any{
		"block_map": map[string]any{
			"page_1": blockData(map[string]any{
				"type":     "page",
				"children": []any{"image_1"},
			}),
			"image_1": blockData(map[string]any{
				"type":      "image",
				"parent_id": "page_1",
				"image": map[string]any{
					"token":    token,
					"name":     "architecture.png",
					"mimeType": "image/png",
					"width":    1200,
					"height":   800,
					"size":     1234,
					"caption":  attributedText("Architecture diagram"),
				},
			}),
		},
	}
	received := []ImageReference{}

	result := ConvertClientVarsWithOptions(clientVars, "page_1", Options{
		ResolveImage: func(reference ImageReference) ImageResolution {
			received = append(received, reference)
			return ImageResolution{
				MarkdownPath: "assets/docx_1/image-001.png",
				AltText:      "Architecture diagram",
				Asset: map[string]any{
					"path":      "assets/docx_1/image-001.png",
					"mimeType":  "image/png",
					"width":     1200,
					"height":    800,
					"sizeBytes": 1234,
					"status":    "downloaded",
					"ordinal":   1,
				},
			}
		},
	})

	if result.Markdown != "![Architecture diagram](assets/docx_1/image-001.png)\n" {
		t.Fatalf("markdown = %q", result.Markdown)
	}
	if len(received) != 1 {
		t.Fatalf("received references = %#v, want one", received)
	}
	reference := received[0]
	if reference.BlockID != "image_1" || reference.Token != token || reference.Name != "architecture.png" ||
		reference.MimeType != "image/png" || reference.Width != 1200 || reference.Height != 800 ||
		reference.DeclaredSize != 1234 || reference.Caption != "Architecture diagram" {
		t.Fatalf("reference = %#v", reference)
	}
	if len(result.Assets) != 1 || result.Assets[0]["path"] != "assets/docx_1/image-001.png" {
		t.Fatalf("assets = %#v", result.Assets)
	}
	assertStringSlice(t, result.Warnings, nil)
	if containsString(result.Markdown, token) || containsAssetValue(result.Assets, token) {
		t.Fatalf("image token leaked in result: %#v", result)
	}
}

func TestConvertClientVarsRejectsUnsafeImageResolverOutput(t *testing.T) {
	token := "raw-image-token"
	clientVars := map[string]any{
		"block_map": map[string]any{
			"page_1": blockData(map[string]any{"type": "page", "children": []any{"image_1"}}),
			"image_1": blockData(map[string]any{
				"type":      "image",
				"parent_id": "page_1",
				"image":     map[string]any{"token": token},
			}),
		},
	}

	result := ConvertClientVarsWithOptions(clientVars, "page_1", Options{
		ResolveImage: func(_ ImageReference) ImageResolution {
			return ImageResolution{
				MarkdownPath: "assets/" + token + "/image-001.png",
				AltText:      "unsafe",
			}
		},
	})

	if result.Markdown != "[image]\n" {
		t.Fatalf("markdown = %q", result.Markdown)
	}
	assertStringSlice(t, result.Warnings, []string{"image resolution rejected unsafe output"})
	if containsString(result.Markdown, token) || containsString(result.Warnings[0], token) {
		t.Fatalf("image token leaked in unsafe result: %#v", result)
	}
}

func TestConvertClientVarsRejectsNestedResolverOutputContainingImageToken(t *testing.T) {
	token := "raw-image-token"
	clientVars := map[string]any{
		"block_map": map[string]any{
			"page_1": blockData(map[string]any{"type": "page", "children": []any{"image_1"}}),
			"image_1": blockData(map[string]any{
				"type":      "image",
				"parent_id": "page_1",
				"image":     map[string]any{"token": token},
			}),
		},
	}

	result := ConvertClientVarsWithOptions(clientVars, "page_1", Options{
		ResolveImage: func(_ ImageReference) ImageResolution {
			return ImageResolution{
				MarkdownPath: "assets/docx_1/image-001.png",
				AltText:      "Architecture diagram",
				Asset: map[string]any{
					"path":   "assets/docx_1/image-001.png",
					"source": map[string]string{"resourceToken": token},
				},
			}
		},
	})

	if result.Markdown != "[image]\n" {
		t.Fatalf("markdown = %q", result.Markdown)
	}
	if len(result.Assets) != 0 {
		t.Fatalf("assets = %#v, want empty", result.Assets)
	}
	assertStringSlice(t, result.Warnings, []string{"image resolution rejected unsafe output"})
}

func TestConvertClientVarsPreservesImageMarkerWhenResolverPanics(t *testing.T) {
	token := "raw-image-token"
	clientVars := map[string]any{
		"block_map": map[string]any{
			"page_1": blockData(map[string]any{"type": "page", "children": []any{"image_1"}}),
			"image_1": blockData(map[string]any{
				"type":      "image",
				"parent_id": "page_1",
				"image":     map[string]any{"token": token},
			}),
		},
	}

	result := ConvertClientVarsWithOptions(clientVars, "page_1", Options{
		ResolveImage: func(_ ImageReference) ImageResolution {
			panic("download failed for " + token)
		},
	})

	if result.Markdown != "[image]\n" {
		t.Fatalf("markdown = %q", result.Markdown)
	}
	if len(result.Assets) != 0 {
		t.Fatalf("assets = %#v, want empty", result.Assets)
	}
	assertStringSlice(t, result.Warnings, []string{"image resolution failed"})
	if containsString(result.Warnings[0], token) {
		t.Fatalf("image token leaked in panic warning: %#v", result.Warnings)
	}
}

func TestConvertClientVarsNumbersOrderedSiblings(t *testing.T) {
	clientVars := map[string]any{
		"block_map": map[string]any{
			"page_1": blockData(map[string]any{
				"type":     "page",
				"children": []any{"ordered_1", "ordered_2"},
			}),
			"ordered_1": blockData(map[string]any{
				"type":      "ordered",
				"parent_id": "page_1",
				"text":      attributedText("First"),
			}),
			"ordered_2": blockData(map[string]any{
				"type":      "ordered",
				"parent_id": "page_1",
				"text":      attributedText("Second"),
			}),
		},
	}

	result := ConvertClientVars(clientVars, "page_1")

	if result.Markdown != "1. First\n\n2. Second\n" {
		t.Fatalf("markdown = %q", result.Markdown)
	}
	assertCounts(t, result.Counts, map[string]int{"page": 1, "ordered": 2})
	assertStringSlice(t, result.Warnings, nil)
}

func TestConvertClientVarsIndentsNestedBulletsAndCalloutBullets(t *testing.T) {
	clientVars := map[string]any{
		"block_map": map[string]any{
			"page_1": blockData(map[string]any{
				"type":     "page",
				"children": []any{"bullet_1", "callout_1"},
			}),
			"bullet_1": blockData(map[string]any{
				"type":      "bullet",
				"parent_id": "page_1",
				"children":  []any{"bullet_2"},
				"text":      attributedText("Parent"),
			}),
			"bullet_2": blockData(map[string]any{
				"type":      "bullet",
				"parent_id": "bullet_1",
				"text":      attributedText("Child"),
			}),
			"callout_1": blockData(map[string]any{
				"type":      "callout",
				"parent_id": "page_1",
				"children":  []any{"bullet_3"},
			}),
			"bullet_3": blockData(map[string]any{
				"type":      "bullet",
				"parent_id": "callout_1",
				"text":      attributedText("Callout child"),
			}),
		},
	}

	result := ConvertClientVars(clientVars, "page_1")

	if result.Markdown != "- Parent\n\n  - Child\n\n[callout]\n\n  - Callout child\n" {
		t.Fatalf("markdown = %q", result.Markdown)
	}
	assertCounts(t, result.Counts, map[string]int{"page": 1, "bullet": 3, "callout": 1})
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

func containsString(value string, needle string) bool {
	return stringsContains(value, needle)
}

func containsAssetValue(values []map[string]any, needle string) bool {
	for _, value := range values {
		for key, item := range value {
			if stringsContains(key, needle) || stringsContains(anyString(item), needle) {
				return true
			}
		}
	}
	return false
}

func anyString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return ""
	}
}

func stringsContains(haystack string, needle string) bool {
	return needle != "" && len(haystack) >= len(needle) && stringsIndex(haystack, needle) >= 0
}

func stringsIndex(haystack string, needle string) int {
	for index := 0; index+len(needle) <= len(haystack); index++ {
		if haystack[index:index+len(needle)] == needle {
			return index
		}
	}
	return -1
}
