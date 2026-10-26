package cli_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/shouhon/rulecheck/internal/cli"
	"github.com/shouhon/rulecheck/internal/report"
	"github.com/shouhon/rulecheck/internal/version"
)

func decodeReport(t *testing.T, output string) report.Report {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(output))
	decoder.DisallowUnknownFields()
	var result report.Report
	if err := decoder.Decode(&result); err != nil {
		t.Fatalf("JSONを読み取れません: %v", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("JSONオブジェクト1件の後に不要な出力があります: %v", err)
	}
	return result
}

func TestScanInvalidArguments(t *testing.T) {
	for _, args := range [][]string{
		{}, {"--rules"}, {"--skills"}, {"--command"},
		{"--rules="}, {"--skills", " \t"}, {"--command="},
		{"--rules", "--json"}, {"--skills", "--command", "rg"},
		{"--rules", "file\x00.md"}, {"--command", "../rg"},
		{"--command", `C:\tools\rg.exe`}, {"--command", "."},
		{"--unknown=模擬秘密値"}, {"--rules", "AGENTS.md", "余分な引数"},
		{"--rules", "AGENTS.md", "--json=false"},
		{"--summary"}, {"--rules", "AGENTS.md", "--summary=true"},
		{"--rules", "--summary"},
		{"--rules", "AGENTS.md", "--help"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			for _, asJSON := range []bool{false, true} {
				input := append([]string{"scan"}, args...)
				if asJSON {
					input = append(input, "--json")
				}
				var stdout, stderr bytes.Buffer
				if code := cli.Run(input, &stdout, &stderr); code != 64 {
					t.Fatalf("不正引数の終了コード = %d", code)
				}
				if !strings.Contains(stderr.String(), "引数が正しくありません") {
					t.Fatalf("引数誤りの日本語診断がありません: %q", stderr.String())
				}
				if strings.Contains(stdout.String()+stderr.String(), "模擬秘密値") {
					t.Fatal("不正な引数の値が出力に漏れています")
				}
				// 元の引数にJSON指定があれば、引数誤りでもJSONを返す。
				if asJSON || slices.Contains(args, "--json") {
					result := decodeReport(t, stdout.String())
					if result.Status != "invalid_arguments" || result.ExitCode() != 64 || result.Counts.Error != 1 {
						t.Fatalf("引数誤りの結果が一致しません: %+v", result)
					}
					if result.Coverage.Rules != report.NotRun || result.Results[0].Findings[0].Code != "invalid_arguments" {
						t.Fatalf("引数誤りを検査済みとして扱っています: %+v", result)
					}
				} else if stdout.Len() != 0 {
					t.Fatalf("通常表示の引数誤りが標準出力へ出ています: %q", stdout.String())
				}
			}
		})
	}
}

func TestScanJSONContract(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	t.Setenv("PATHEXT", ".EXE")
	rules := filepath.Join(dir, "日本語 空白.md")
	other := filepath.Join(dir, "別ファイル.md")
	skills := filepath.Join(dir, "スキル 親")
	for _, path := range []string{rules, other} {
		if err := os.WriteFile(path, []byte("模擬の指示\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(skills, 0700); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := cli.Run([]string{
		"scan", "--rules", rules, "--skills=" + skills,
		"--command", "rg", "--rules=" + other, "--json",
	}, &stdout, &stderr)
	if code != 1 || stderr.Len() != 0 {
		t.Fatalf("CLI不足の検査: 終了コード=%d、標準エラー=%q", code, stderr.String())
	}
	result := decodeReport(t, stdout.String())
	if result.SchemaVersion != 1 || result.Tool.Name != "rulecheck" || result.Tool.Version != version.Current || result.Command != "scan" {
		t.Fatalf("JSONの識別情報が一致しません: %+v", result)
	}
	if result.Status != "failed" || result.ExitCode() != code || result.Counts != (report.Counts{Error: 1}) || len(result.Results) != 4 {
		t.Fatalf("状態・件数・終了コードが一致しません: %+v", result)
	}
	if result.Coverage != (report.Coverage{
		Rules: report.Completed, References: report.Completed,
		Skills: report.Completed, Commands: report.Completed,
	}) {
		t.Fatalf("実施した範囲が一致しません: %+v", result.Coverage)
	}
	for _, target := range result.Results {
		if target.Kind != "command" && len(target.Findings) == 0 {
			if target.ResolvedPath == "" || len(target.Aliases) != 1 || target.Aliases[0] != target.Target {
				t.Fatalf("実体と別名が一致しません: %+v", target)
			}
			continue
		}
		if target.Kind != "command" || len(target.Findings) != 1 {
			t.Fatalf("対象別の結果がありません: %+v", target)
		}
		finding := target.Findings[0]
		wantCategory, wantCode := report.Error, "command_missing"
		if finding.Category != wantCategory || finding.Code != wantCode || finding.Message == "" || finding.Paths == nil {
			t.Fatalf("診断情報が一致しません: %+v", finding)
		}
	}
}

func TestScanDeterministicOutput(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("PATHEXT", ".EXE")
	inputs := [][]string{
		{"scan", "--rules", "b.md", "--rules", "a.md", "--command", "rg", "--skills", "skills", "--json"},
		{"scan", "--json", "--skills=skills", "--command=rg", "--rules=a.md", "--rules=b.md", "--rules=a.md", "--command=rg"},
	}
	var outputs []string
	for _, args := range inputs {
		var stdout, stderr bytes.Buffer
		if code := cli.Run(args, &stdout, &stderr); code != 1 || stderr.Len() != 0 {
			t.Fatalf("検査結果を出力できません: 終了コード=%d、標準エラー=%q", code, stderr.String())
		}
		outputs = append(outputs, stdout.String())
	}
	if outputs[0] != outputs[1] {
		t.Fatalf("指定順と完全に同じ対象の再指定で出力が変わりました:\n%s\n%s", outputs[0], outputs[1])
	}
}

func TestScanUnrequestedCoverage(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("PATHEXT", ".EXE")
	var stdout, stderr bytes.Buffer
	cli.Run([]string{"scan", "--command=模擬CLI", "--json"}, &stdout, &stderr)
	result := decodeReport(t, stdout.String())
	if result.Coverage != (report.Coverage{
		Rules: report.NotRequested, References: report.NotRequested,
		Skills: report.NotRequested, Commands: report.Completed,
	}) {
		t.Fatalf("指定していない範囲が検査対象になっています: %+v", result.Coverage)
	}
}

func TestScanJSONOptionIsNotInferredFromValue(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"scan", "--rules=--json"}, &stdout, &stderr); code != 1 {
		t.Fatalf("終了コード = %d", code)
	}
	if !strings.HasPrefix(stdout.String(), "エラーあり\n") || stderr.Len() != 0 {
		t.Fatalf("パスをJSON指定と誤認しています: %q %q", stdout.String(), stderr.String())
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("模擬の出力失敗")
}

func TestScanOutputFailure(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("PATHEXT", ".EXE")
	for _, asJSON := range []bool{false, true} {
		args := []string{"scan", "--command=rg"}
		if asJSON {
			args = append(args, "--json")
		}
		var stderr bytes.Buffer
		if code := cli.Run(args, failingWriter{}, &stderr); code != 2 || !strings.Contains(stderr.String(), "結果を出力できません") {
			t.Fatalf("出力失敗が通知されません: 終了コード=%d、標準エラー=%q", code, stderr.String())
		}
	}
}
