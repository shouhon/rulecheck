//go:build !windows

package scan

import "io/fs"

func commandNames(name string) ([]string, bool) { return []string{name}, true }

func commandFile(info fs.FileInfo) bool {
	return info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0
}
