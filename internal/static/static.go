package static

import _ "embed"

// SkillMd contains the embedded skill.md file for AI agents.
//
//go:embed skill.md
var SkillMd string

// IndexHTML contains the embedded index.html landing page.
//
//go:embed index.html
var IndexHTML string
