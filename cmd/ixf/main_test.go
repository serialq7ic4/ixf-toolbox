package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionCommandPrintsUnifiedCLIName(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"--version"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr.String())
	}
	want := "ixf " + version
	if strings.TrimSpace(stdout.String()) != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestRootHelpListsCommands(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"--help"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr.String())
	}
	for _, expected := range []string{"usage: ixf", "docs", "okr", "update"} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("stdout missing %q: %s", expected, stdout.String())
		}
	}
}

func TestDocsAndOKRHelpListSupportedSubcommands(t *testing.T) {
	tests := []struct {
		args     []string
		expected []string
	}{
		{args: []string{"docs", "--help"}, expected: []string{"usage: ixf docs", "read", "publish", "inspect"}},
		{args: []string{"okr", "--help"}, expected: []string{"usage: ixf okr", "read", "write"}},
	}
	for _, test := range tests {
		var stdout bytes.Buffer
		var stderr bytes.Buffer

		code := run(test.args, &stdout, &stderr)

		if code != 0 {
			t.Fatalf("run(%v) exit code = %d, want 0; stderr=%q", test.args, code, stderr.String())
		}
		for _, expected := range test.expected {
			if !strings.Contains(stdout.String(), expected) {
				t.Fatalf("run(%v) stdout missing %q: %s", test.args, expected, stdout.String())
			}
		}
	}
}
