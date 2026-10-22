//go:build !windows

package scan

import (
	"io/fs"
	"path/filepath"
	"strings"
)

func localAbs(path string) (string, error) {
	if strings.HasPrefix(path, "//") || strings.HasPrefix(path, `\\`) || strings.Contains(path, "://") {
		return "", errNonLocal
	}
	return filepath.Abs(path)
}

func isLink(info fs.FileInfo) bool {
	return info.Mode()&fs.ModeSymlink != 0
}

func pathKey(path string) string { return path }
