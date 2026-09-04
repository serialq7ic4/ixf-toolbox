package ixftoolbox

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var rawVersion string

var DefaultVersion = strings.TrimSpace(rawVersion)
