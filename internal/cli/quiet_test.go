package cli_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/shouhon/rulecheck/internal/cli"
	"github.com/shouhon/rulecheck/internal/report"
)

func TestScanQuiet(t *testing.T) {
	dir := t.TempDir()
	skills := filepath.Join(dir, "skills")
	for i := range 50 {
		name := fmt.Sprintf("skill-%02d", i)
		path := filepath.Join(skills, name)
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
		body := "---\nname: " + name + "\ndescription: 模擬秘密値\n---\n[外部](https://example.invalid/模擬秘密値)\n"
		if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	warning := filepath.Join(dir, "warning")
	if err := os.Mkdir(warning, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(warning, "SKILL.md"), []byte("---\nname: other\ndescription: 説明\n---\n"), 0600); err != nil {
		t.Fatal(err)
	}
	unknown := filepath.Join(dir, "unknown.md")
	if err := os.WriteFile(unknown, []byte("[[独自参照]]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		args []string
		code int
	}{
		{"正常50件", nil, 0},
		{"注意", []string{"--skill", warning}, 0},
		{"エラー", []string{"--rules", filepath.Join(dir, "missing.md")}, 1},
		{"判定不能を優先", []string{"--rules", unknown, "--rules", filepath.Join(dir, "missing.md")}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := func(flags ...string) string {
				t.Helper()
				args := append([]string{"scan", "--skills", skills}, tc.args...)
				args = append(args, flags...)
				var stdout, stderr bytes.Buffer
				if code := cli.Run(args, &stdout, &stderr); code != tc.code || stderr.Len() != 0 {
					t.Fatalf("終了コード=%d、標準エラー=%q", code, stderr.String())
				}
				if strings.Contains(stdout.String(), "模擬秘密値") {
					t.Fatal("本文やメタデータの値が出力されています")
				}
				return stdout.String()
			}
			fullJSON := run("--json")
			full := decodeReport(t, fullJSON)
			if strings.Contains(fullJSON, "target_count") {
				t.Fatal("通常のJSONに省略表示専用の項目があります")
			}
			expected := make([]report.Result, 0)
			for _, result := range full.Results {
				result.Findings = slices.DeleteFunc(slices.Clone(result.Findings), func(f report.Finding) bool { return f.Category == report.Skipped })
				if len(result.Findings) > 0 {
					expected = append(expected, result)
				}
			}
			quietJSON := run("--json", "--quiet")
			quiet := decodeReport(t, quietJSON)
			if quiet.TargetCount == nil || *quiet.TargetCount != len(full.Results) || quiet.Counts != full.Counts || quiet.Status != full.Status || quiet.Coverage != full.Coverage || quiet.ExitCode() != full.ExitCode() {
				t.Fatalf("省略表示で検査対象数・件数・判定・範囲が変わっています: %+v", quiet)
			}
			if !reflect.DeepEqual(quiet.Results, expected) || !reflect.DeepEqual(quiet.SkippedSummary, []report.SkippedCount{{Code: "reference_external_skipped", Count: 50}}) {
				t.Fatalf("問題の詳細や対象外の件数が一致しません: %+v", quiet)
			}
			if quietJSON != run("--quiet", "--summary", "--json") || quietJSON != run("--summary", "--json", "--quiet", "--quiet") {
				t.Fatal("--summaryとの併用や指定順で省略表示が変わっています")
			}
			text := run("--quiet")
			if strings.Contains(text, "[対象外]") || strings.Contains(text, strconv.Quote(skills)) || strings.Contains(text, "skill-00") || !strings.Contains(text, fmt.Sprintf("検査対象: %d件\n", len(full.Results))) || !strings.Contains(text, "対象外 50\n") {
				t.Fatalf("正常項目が残っているか件数がありません: %s", text)
			}
			for _, result := range quiet.Results {
				for _, finding := range result.Findings {
					if !strings.Contains(text, finding.Code+": "+finding.Message) {
						t.Fatalf("通常表示から問題の詳細が消えています: %s", finding.Code)
					}
				}
			}
			if tc.name == "正常50件" && (strings.Contains(quietJSON, `"results":null`) || strings.Count(text, "\n") > 6) {
				t.Fatalf("正常時の出力が空配列か簡潔な表示になっていません: %s", text)
			}
		})
	}
}

func TestScanQuietValueIsNotAnOption(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := cli.Run([]string{"scan", "--rules=--quiet", "--json"}, &stdout, &stderr)
	result := decodeReport(t, stdout.String())
	if code != 1 || stderr.Len() != 0 || result.TargetCount != nil || len(result.Results) != 1 {
		t.Fatalf("パスを省略表示の指定と誤認しています: %+v", result)
	}
}
