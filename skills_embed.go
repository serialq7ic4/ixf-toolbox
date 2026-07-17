package ixftoolbox

import "embed"

// SkillFS contains the agent skills installed by the Go ixf runtime.
//
//go:embed skills/*/*/SKILL.md
var SkillFS embed.FS
