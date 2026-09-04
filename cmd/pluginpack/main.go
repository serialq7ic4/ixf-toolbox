package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/serialq7ic4/ixf-toolbox/internal/pluginpack"
)

func main() {
	check := false
	for _, arg := range os.Args[1:] {
		switch arg {
		case "--check":
			check = true
		case "", "--help", "-h":
			if arg != "" {
				fmt.Println("usage: go run ./cmd/pluginpack [--check]")
				return
			}
		default:
			fmt.Fprintf(os.Stderr, "ERROR unsupported pluginpack flag: %s\n", arg)
			os.Exit(2)
		}
	}
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR resolve repository root: %s\n", err)
		os.Exit(1)
	}
	if err := pluginpack.Generate(pluginpack.Options{Root: filepath.Clean(root), Check: check}); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR %s\n", err)
		os.Exit(1)
	}
}
