package web

import (
	"embed"
	"io/fs"
)

//go:embed index.html static
var assets embed.FS

func Assets() fs.FS {
	return assets
}
