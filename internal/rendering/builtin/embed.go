package builtin

import (
	"embed"
	"io/fs"
)

//go:embed templates
var assets embed.FS

func Files() fs.FS { root, _ := fs.Sub(assets, "templates"); return root }
