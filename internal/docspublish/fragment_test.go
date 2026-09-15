package docspublish

import (
	"strings"
	"testing"
)

func TestParseMarkdownFragmentAttachesConsecutiveFencesToOrderedItems(t *testing.T) {
	fence := "```"
	markdown := "1. First\n\n  " + fence + "Plain\n  command-one\n  " + fence + "\n\n  " + fence + "Plain\n  command-two\n  " + fence + "\n\n1. Second\n\n" + fence + "bash\ncommand-three\n" + fence + "\n"
	specs, err := ParseMarkdownFragment(markdown)
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 2 || specs[0].Kind != "ordered" || specs[1].Kind != "ordered" {
		t.Fatalf("specs = %#v, want two top-level ordered specs", specs)
	}
	if len(specs[0].Children) != 2 || specs[0].Children[0].Kind != "code" || specs[0].Children[1].Kind != "code" {
		t.Fatalf("first children = %#v, want two code children", specs[0].Children)
	}
	if specs[0].Children[0].Text != "command-one" || specs[0].Children[1].Text != "command-two" {
		t.Fatalf("first child text = %#v", specs[0].Children)
	}
	if len(specs[1].Children) != 1 || specs[1].Children[0].Text != "command-three" {
		t.Fatalf("second children = %#v, want command-three", specs[1].Children)
	}
}

func TestParseMarkdownFragmentAttachesMermaidFenceAsImageChild(t *testing.T) {
	fence := "```"
	markdown := "1. Diagram\n\n" + fence + "Plain\nflowchart LR\n  A --> B\n" + fence + "\n"
	specs, err := ParseMarkdownFragment(markdown)
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 1 || len(specs[0].Children) != 1 {
		t.Fatalf("specs = %#v, want one ordered child", specs)
	}
	child := specs[0].Children[0]
	if child.Kind != "image" || child.SourceKind != "mermaid" || !strings.Contains(child.Text, "flowchart LR") {
		t.Fatalf("mermaid child = %#v", child)
	}
}

func TestParseMarkdownFragmentStopsOrderedChildrenAtNonFence(t *testing.T) {
	fence := "```"
	markdown := "1. First\n\n" + fence + "Plain\ncommand-one\n" + fence + "\n\nBody stops the child run.\n\n" + fence + "Plain\nroot-code\n" + fence + "\n"
	specs, err := ParseMarkdownFragment(markdown)
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 3 || len(specs[0].Children) != 1 || specs[1].Kind != "text" || specs[2].Kind != "code" {
		t.Fatalf("specs = %#v, want ordered, text, code", specs)
	}
}

func TestParseMarkdownFragmentAcceptsReaderIndentedFence(t *testing.T) {
	fence := "```"
	markdown := "1. Step\n\n  " + fence + "Plain\n  echo ready\n  " + fence + "\n"
	specs, err := ParseMarkdownFragment(markdown)
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 1 || len(specs[0].Children) != 1 || specs[0].Children[0].Text != "echo ready" {
		t.Fatalf("specs = %#v, want one de-indented code child", specs)
	}
}

func TestParseMarkdownFragmentDoesNotUseOrderedNumbersForGrouping(t *testing.T) {
	fence := "```"
	markdown := "1. First\n\n1. Second\n\n" + fence + "Plain\nsecond-code\n" + fence + "\n"
	specs, err := ParseMarkdownFragment(markdown)
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 2 || specs[0].Kind != "ordered" || specs[1].Kind != "ordered" || len(specs[1].Children) != 1 {
		t.Fatalf("specs = %#v, want two ordered specs with the second child", specs)
	}
}

