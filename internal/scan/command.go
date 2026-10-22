package scan

import (
	"os"
	"path/filepath"
	"strings"

	"rulecheck/internal/report"
)

// checkCommand はPATHにあるローカルの候補を調べ、本文の読取も起動も行わない。
func checkCommand(name string) report.Result {
	result := report.Result{Kind: "command", Target: name}
	if strings.TrimSpace(name) == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\:\x00") {
		result.Findings = []report.Finding{commandUnknown()}
		return result
	}
	names, complete := commandNames(name)
	for _, entry := range filepath.SplitList(os.Getenv("PATH")) {
		// Windowsで暗黙に検索される作業ディレクトリは追加しない。
		if entry == "" {
			continue
		}
		dir, err := resolve(entry)
		if err != nil {
			if pathFinding(err).Category == report.Unknown {
				complete = false
			}
			continue
		}
		if !dir.info.IsDir() {
			continue
		}
		for _, candidate := range names {
			file, err := resolve(filepath.Join(dir.path, candidate))
			if err != nil {
				if pathFinding(err).Category == report.Unknown {
					complete = false
				}
				continue
			}
			if commandFile(file.info) {
				result.ResolvedPath = file.path
				result.Links = append(dir.links, file.links...)
				return result
			}
		}
	}
	if !complete {
		result.Findings = []report.Finding{commandUnknown()}
	} else {
		result.Findings = []report.Finding{{
			Code: "command_missing", Category: report.Error,
			Message: "現在のPATHにCLIの実行ファイル候補が見つかりません。",
		}}
	}
	return result
}

func commandUnknown() report.Finding {
	return report.Finding{
		Code: "command_unresolved", Category: report.Unknown,
		Message: "CLIの候補または検索設定を確認できません。ネットワーク上や特殊なパスは開きません。",
	}
}
