package skill

import (
	"embed"
)

//go:embed packages/*/SKILL.md
var builtinFS embed.FS
