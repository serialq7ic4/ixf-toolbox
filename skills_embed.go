package ixftoolbox

import "embed"

// SkillFS contains the same agent skills installed by the Python runtime.
//
//go:embed skills/*/*/SKILL.md
var SkillFS embed.FS
