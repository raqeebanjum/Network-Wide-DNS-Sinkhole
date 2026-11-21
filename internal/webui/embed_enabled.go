//go:build embedded

package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var embeddedFiles embed.FS

func frontend() (fs.FS, bool) {
	content, err := fs.Sub(embeddedFiles, "dist")
	if err != nil {
		return nil, false
	}
	return content, true
}
