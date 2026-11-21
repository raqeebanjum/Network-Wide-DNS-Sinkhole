//go:build !embedded

package webui

import "io/fs"

func frontend() (fs.FS, bool) {
	return nil, false
}
