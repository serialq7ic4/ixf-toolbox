package servicedeskapi

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestTransportPackageDoesNotDependOnProfileOrRuntime(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, item := range file.Imports {
			path, _ := strconv.Unquote(item.Path.Value)
			if strings.Contains(path, "/pkg/servicedeskprofile") || strings.Contains(path, "/internal/servicedesk") {
				t.Errorf("%s imports forbidden runtime package %q", name, path)
			}
		}
	}
}
