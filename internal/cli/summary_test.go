package cli_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/shouhon/rulecheck/internal/cli"
	"github.com/shouhon/rulecheck/internal/report"
)

func TestScanSummary(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.md"), filepath.Join(dir, "b.md")
	plain, unknown := filepath.Join(dir, "plain.md"), filepath.Join(dir, "unknown.md")
	skills := filepath.Join(dir, "skills")
	skill := filepath.Join(skills, "actual", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skill), 0700); err != nil {
		t.Fatal(err)
	}
	for path, body := range map[string]string{
		a:     strings.Repeat("[外部](https://example.invalid/模擬秘密値)\n", 350) + "[見出し](#見出し)\n\n`例`\n",
		b:     strings.Repeat("[外部](https://example.invalid/模擬秘密値)\n", 350) + "\n```\n例\n```\n",
		plain: "模擬の指示\n", unknown: "[[独自参照]]\n",
		skill: "---\nname: other\ndescription: 模擬の説明\n---\n",
	} {
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	base := []string{"--rules", a, "--rules", b}
	for _, tc := range []struct {
		name   string
		args   []string
		counts report.Counts
		status string
		code   int
	}{
		{"対象外703件", base, report.Counts{Skipped: 703}, "passed", 0},
		{"エラー併存", append(slices.Clone(base), "--rules="+filepath.Join(dir, "missing.md")), report.Counts{Error: 1, Skipped: 703}, "failed", 1},
		{"注意併存", append(slices.Clone(base), "--skills="+skills), report.Counts{Warning: 1, Skipped: 703}, "warning", 0},
		{"判定不能を優先", append(slices.Clone(base), "--rules="+unknown, "--rules="+filepath.Join(dir, "missing.md")), report.Counts{Error: 1, Unknown: 1, Skipped: 703}, "incomplete", 2},
		{"対象外0件", []string{"--rules", plain}, report.Counts{}, "passed", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := func(flags ...string) string {
				t.Helper()
				args := append([]string{"scan"}, tc.args...)
				args = append(args, flags...)
				var stdout, stderr bytes.Buffer
				if code := cli.Run(args, &stdout, &stderr); code != tc.code || stderr.Len() != 0 {
					t.Fatalf("終了コード=%d、標準エラー=%q", code, stderr.String())
				}
				if strings.Contains(stdout.String(), "模擬秘密値") {
					t.Fatal("本文の模擬秘密値が出力されています")
				}
				return stdout.String()
			}
			fullJSON := run("--json")
			full := decodeReport(t, fullJSON)
			summaryJSON := run("--json", "--summary")
			summary := decodeReport(t, summaryJSON)
			if full.Counts != tc.counts || summary.Counts != tc.counts || summary.Status != tc.status || summary.ExitCode() != tc.code {
				t.Fatalf("要約前後の件数・状態・終了コードが一致しません: %+v", summary)
			}
			if strings.Contains(fullJSON, "skipped_summary") {
				t.Fatal("従来のJSONに要約専用の項目が追加されています")
			}
			var want []report.SkippedCount
			if tc.counts.Skipped > 0 {
				want = []report.SkippedCount{
					{Code: "reference_code_skipped", Count: 2},
					{Code: "reference_external_skipped", Count: 700},
					{Code: "reference_fragment_skipped", Count: 1},
				}
			}
			if !reflect.DeepEqual(summary.SkippedSummary, want) {
				t.Fatalf("種類別の集計が一致しません: %+v", summary.SkippedSummary)
			}
			skipped := 0
			for i := range full.Results {
				full.Results[i].Findings = slices.DeleteFunc(full.Results[i].Findings, func(finding report.Finding) bool {
					if finding.Category == report.Skipped {
						skipped++
						return true
					}
					return false
				})
			}
			summary.SkippedSummary = nil
			if skipped != tc.counts.Skipped || !reflect.DeepEqual(full, summary) {
				t.Fatal("対象外以外の診断・対象・実体・検査範囲などが要約で変わっています")
			}
			if strings.Contains(summaryJSON, `"findings":null`) || strings.Contains(summaryJSON, `"category":"skipped"`) {
				t.Fatal("要約した診断が空配列になっていないか、対象外の詳細が残っています")
			}
			text := run("--summary")
			if strings.Contains(text, "[対象外]") || !strings.Contains(text, fmt.Sprintf("対象外 %d\n", tc.counts.Skipped)) {
				t.Fatal("通常表示に対象外の詳細が残っているか、総件数が異なります")
			}
			for _, count := range want {
				if !strings.Contains(text, fmt.Sprintf("  %s: %d件\n", count.Code, count.Count)) {
					t.Fatalf("通常表示に対象外の種類別件数がありません: %s", count.Code)
				}
			}
			for _, result := range summary.Results {
				for _, finding := range result.Findings {
					if !strings.Contains(text, finding.Code+": "+finding.Message) {
						t.Fatalf("通常表示から問題の詳細が消えています: %s", finding.Code)
					}
				}
			}
		})
	}
}

func TestScanSummaryOptionIsNotInferredFromValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "AGENTS.md")
	if err := os.WriteFile(path, []byte("[見出し](#見出し)\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := cli.Run([]string{"scan", "--rules=" + path, "--rules=--summary"}, &stdout, &stderr)
	if code != 1 || stderr.Len() != 0 || !strings.Contains(stdout.String(), "[対象外] reference_fragment_skipped:") {
		t.Fatalf("パスを要約指定と誤認しています: 終了コード=%d、標準出力=%q、標準エラー=%q", code, stdout.String(), stderr.String())
	}
}
