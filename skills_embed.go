package ixftoolbox

import (
	"embed"
	"strings"
)

// SkillFS contains the agent skills installed by the Go ixf runtime.
//
//go:embed skills/*/*/SKILL.md
var SkillFS embed.FS

//go:embed VERSION
var rawVersion string

// DefaultVersion is the source-tree CLI version used when release builds do not
// override main.version through ldflags.
var DefaultVersion = strings.TrimSpace(rawVersion)
