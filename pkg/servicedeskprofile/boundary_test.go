package servicedeskprofile

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func TestPublicPackageImportsStayPure(t *testing.T) {
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
			path, err := strconv.Unquote(item.Path.Value)
			if err != nil {
				t.Fatalf("unquote import in %s: %v", name, err)
			}
			if strings.Contains(path, ".") && path != "github.com/serialq7ic4/ixf-toolbox/internal/servicedeskapi" {
				t.Errorf("%s imports non-whitelisted package %q", name, path)
			}
		}
		ast.Inspect(file, func(ast.Node) bool { return true })
	}
}

func TestMetadataClientExportedMethodSet(t *testing.T) {
	typeOf := reflect.TypeOf((*MetadataClient)(nil))
	methods := make([]string, 0, typeOf.NumMethod())
	for index := 0; index < typeOf.NumMethod(); index++ {
		methods = append(methods, typeOf.Method(index).Name)
	}
	sort.Strings(methods)
	want := []string{"Lading", "Service"}
	if !reflect.DeepEqual(methods, want) {
		t.Fatalf("MetadataClient exported methods = %v, want %v", methods, want)
	}
}
