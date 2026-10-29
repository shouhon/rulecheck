package cli_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shouhon/rulecheck/internal/cli"
	"github.com/shouhon/rulecheck/internal/report"
)

func TestScanReferenceCounts(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "target.md"), []byte("模擬秘密値\x00"), 0600); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "RULES.md")
	for _, tc := range []struct {
		name   string
		body   string
		counts report.ReferenceCounts
		code   int
	}{
		{"参照なし", "説明だけ\n", report.ReferenceCounts{}, 0},
		{"Issueの最小例", "[正常](target.md)\n\n`src/sample.py` は説明用のコードです。\n", report.ReferenceCounts{Checked: 1, Passed: 1, Skipped: 1}, 0},
		{"全件対象外", "[外部](https://example.invalid/模擬秘密値)\n[見出し](#見出し)\n\n`例`\n", report.ReferenceCounts{Skipped: 3}, 0},
		{"不存在", "[不足](missing.md)\n", report.ReferenceCounts{Checked: 1, Error: 1}, 1},
		{"判定不能併存", "[正常](target.md)\n[不足](missing.md)\n[未対応](file:///C:/hidden.md)\n\n[[独自記法]]\n\n<img src=\"unknown.png\">\n", report.ReferenceCounts{Checked: 5, Passed: 1, Error: 1, Unknown: 3}, 2},
		{"見出し付きのパスは成功", "[正常](target.md#見出し)\n", report.ReferenceCounts{Checked: 1, Passed: 1, Skipped: 1}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(file, []byte(tc.body), 0600); err != nil {
				t.Fatal(err)
			}
			var full report.Report
			for i, flags := range [][]string{nil, {"--summary"}, {"--quiet"}, {"--summary", "--quiet"}} {
				args := append([]string{"scan", "--rules", file}, flags...)
				var stdout, stderr bytes.Buffer
				code := cli.Run(append(args, "--json"), &stdout, &stderr)
				result := decodeReport(t, stdout.String())
				if code != tc.code || stderr.Len() != 0 || result.ReferenceCounts != tc.counts || !strings.Contains(stdout.String(), `"reference_counts":{`) {
					t.Fatalf("表示指定%vの参照件数が一致しません: 終了コード=%d、結果=%+v、標準エラー=%q", flags, code, result, stderr.String())
				}
				if result.ReferenceCounts.Checked != result.ReferenceCounts.Passed+result.ReferenceCounts.Error+result.ReferenceCounts.Unknown {
					t.Fatal("参照の検査対象数と結果の合計が一致しません")
				}
				if i == 0 {
					full = result
				} else if result.Status != full.Status || result.Counts != full.Counts || result.Coverage != full.Coverage {
					t.Fatal("表示指定で判定・分類別件数・検査範囲が変わっています")
				}
				if tc.counts.Error+tc.counts.Unknown == 0 && len(flags) > 0 {
					for _, target := range result.Results {
						if len(target.Findings) != 0 {
							t.Fatal("正常な参照の詳細を増やしています")
						}
					}
				}
				jsonOutput := stdout.String()
				stdout.Reset()
				stderr.Reset()
				code = cli.Run(args, &stdout, &stderr)
				want := fmt.Sprintf("参照: 検査対象 %d / 成功 %d / エラー %d / 判定不能 %d / 対象外 %d\n", tc.counts.Checked, tc.counts.Passed, tc.counts.Error, tc.counts.Unknown, tc.counts.Skipped)
				if code != tc.code || stderr.Len() != 0 || !strings.Contains(stdout.String(), want) {
					t.Fatalf("通常表示の件数・終了コードが一致しません: %s", stdout.String())
				}
				if strings.Contains(jsonOutput+stdout.String(), "模擬秘密値") {
					t.Fatal("参照先の本文や対象外のURLを出力しています")
				}
			}
		})
	}
}

func TestReferenceCountsWithoutScannedBody(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	t.Setenv("PATHEXT", ".EXE")
	for _, tc := range []struct {
		name     string
		args     []string
		coverage string
	}{
		{"CLIだけ", []string{"--command=missing"}, report.NotRequested},
		{"記述元が不在", []string{"--rules=" + filepath.Join(dir, "missing.md")}, report.Partial},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"scan", "--json", "--quiet"}, tc.args...)
			var stdout, stderr bytes.Buffer
			code := cli.Run(args, &stdout, &stderr)
			result := decodeReport(t, stdout.String())
			if code != 1 || stderr.Len() != 0 || result.ReferenceCounts != (report.ReferenceCounts{}) || result.Coverage.References != tc.coverage {
				t.Fatalf("本文を検査できない場合の件数と検査範囲が一致しません: %+v", result)
			}
		})
	}
}
