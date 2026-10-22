package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"rulecheck/internal/report"
)

func writeReport(result report.Report, asJSON bool, stdout, stderr io.Writer) int {
	var err error
	if asJSON {
		err = json.NewEncoder(stdout).Encode(result)
	} else {
		var text strings.Builder
		status := map[string]string{
			"passed": "検査完了", "warning": "注意あり", "failed": "エラーあり", "incomplete": "検査未完了",
		}
		categories := map[report.Category]string{
			report.Error: "エラー", report.Warning: "注意", report.Unknown: "判定不能", report.Skipped: "対象外",
		}
		kinds := map[string]string{"rules": "指示ファイル", "skills": "スキルの親ディレクトリ", "command": "CLI"}
		fmt.Fprintln(&text, status[result.Status])
		for _, target := range result.Results {
			fmt.Fprintf(&text, "対象: %s %q\n", kinds[target.Kind], target.Target)
			for _, finding := range target.Findings {
				fmt.Fprintf(&text, "  [%s] %s: %s\n", categories[finding.Category], finding.Code, finding.Message)
			}
		}
		fmt.Fprintf(&text, "件数: エラー %d / 注意 %d / 判定不能 %d / 対象外 %d\n",
			result.Counts.Error, result.Counts.Warning, result.Counts.Unknown, result.Counts.Skipped)
		_, err = io.WriteString(stdout, text.String())
	}
	if err != nil {
		fmt.Fprintln(stderr, "検査結果を出力できませんでした。")
		return 2
	}
	return result.ExitCode()
}
