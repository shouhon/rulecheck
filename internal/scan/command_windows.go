package scan

import (
	"io/fs"
	"os"
	"strings"
)

func commandNames(name string) ([]string, bool) {
	var names []string
	if strings.Contains(name, ".") {
		names = append(names, name)
	}
	extensions := os.Getenv("PATHEXT")
	if extensions == "" {
		extensions = ".COM;.EXE;.BAT;.CMD"
	}
	complete := true
	for ext := range strings.SplitSeq(strings.ToLower(extensions), ";") {
		if ext == "" {
			continue
		}
		// 拡張子の設定でパスを組み立て直したり、別の領域へ移動させない。
		if strings.ContainsAny(ext, "/\\:*?\"<>|\x00") || strings.TrimRight(ext, ". ") != ext {
			complete = false
			continue
		}
		if ext[0] != '.' {
			ext = "." + ext
		}
		names = append(names, name+ext)
	}
	return names, complete
}

func commandFile(info fs.FileInfo) bool { return info.Mode().IsRegular() }
