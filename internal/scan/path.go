package scan

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/shouhon/rulecheck/internal/report"
)

var (
	errCycle     = errors.New("リンクが循環しています")
	errBroken    = errors.New("リンクの参照先が存在しません")
	errLinkLimit = errors.New("リンクの解決回数が上限を超えました")
	errNonLocal  = errors.New("ローカルの通常パスではありません")
)

type resolved struct {
	path  string
	info  fs.FileInfo
	links []report.Link
}

// resolve は各構成要素を確認し、リンク先を開く前に接続先と循環を検査する。
func resolve(path string) (resolved, error) {
	var result resolved
	path, err := localAbs(path)
	if err != nil {
		return result, err
	}
	steps := 0
	result.path, err = resolvePath(path, make(map[string]bool), &steps, &result.links)
	if err == nil {
		result.info, err = os.Stat(result.path)
	}
	return result, err
}

func resolvePath(path string, active map[string]bool, steps *int, links *[]report.Link) (string, error) {
	path, err := localAbs(path)
	if err != nil {
		return "", err
	}
	volume := filepath.VolumeName(path)
	current := volume + string(filepath.Separator)
	parts := strings.FieldsFunc(path[len(volume):], func(r rune) bool { return r == '/' || r == rune(filepath.Separator) })
	for _, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return "", err
		}
		if !isLink(info) {
			continue
		}
		key := pathKey(current)
		if active[key] {
			return "", errCycle
		}
		*steps++
		if *steps > 255 {
			return "", errLinkLimit
		}
		target, err := os.Readlink(current)
		if err != nil {
			return "", err
		}
		*links = append(*links, report.Link{Path: current, Target: target})
		if !filepath.IsAbs(target) && filepath.VolumeName(target) == "" {
			target = filepath.Join(filepath.Dir(current), target)
		}
		active[key] = true
		current, err = resolvePath(target, active, steps, links)
		delete(active, key)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				err = errors.Join(errBroken, err)
			}
			return "", err
		}
	}
	return current, nil
}

func pathFinding(err error) report.Finding {
	f := report.Finding{Code: "path_unreadable", Category: report.Unknown, Message: "対象または参照先を読み取れません。"}
	var pathError *fs.PathError
	switch {
	case errors.Is(err, errCycle):
		f.Code, f.Category, f.Message = "link_cycle", report.Error, "共有リンクが循環しています。"
	case errors.Is(err, errLinkLimit):
		f.Code, f.Message = "link_limit", "共有リンクの解決回数が上限を超えたため、実体を確認できません。"
	case errors.Is(err, errNonLocal):
		f.Code, f.Message = "path_not_local", "ネットワーク接続や特殊なパスを避けるため、この対象は検査できません。"
	case errors.As(err, &pathError) && pathError.Op == "readlink":
		f.Message = "共有リンクの参照先を解釈できません。"
	case errors.Is(err, errBroken):
		f.Code, f.Category, f.Message = "link_broken", report.Error, "共有リンクの参照先が存在しません。"
	case errors.Is(err, fs.ErrNotExist):
		f.Code, f.Category, f.Message = "path_missing", report.Error, "指定された対象が存在しません。"
	}
	return f
}
