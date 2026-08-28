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

// DefaultVersion is the CLI version embedded from the repository VERSION file.
var DefaultVersion = strings.TrimSpace(rawVersion)
