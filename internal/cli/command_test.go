package cli_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"rulecheck/internal/cli"
	"rulecheck/internal/report"
)

func TestCommandScanOutput(t *testing.T) {
	dir := t.TempDir()
	name := "mock"
	if runtime.GOOS == "windows" {
		name += ".cmd"
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("模擬秘密値は読取も実行もしない\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATHEXT", ".CMD")
	for _, tc := range []struct {
		name, path, command, status, text string
		code                              int
		counts                            report.Counts
	}{
		{"候補あり", dir, "mock", "passed", "検査完了", 0, report.Counts{}},
		{"候補なし", dir, "missing", "failed", "エラーあり", 1, report.Counts{Error: 1}},
		{"判定不能", `\\invalid.example\share`, "mock", "incomplete", "検査未完了", 2, report.Counts{Unknown: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("PATH", tc.path)
			for _, asJSON := range []bool{false, true} {
				args := []string{"scan", "--command", tc.command}
				if asJSON {
					args = append(args, "--json")
				}
				var stdout, stderr bytes.Buffer
				code := cli.Run(args, &stdout, &stderr)
				if code != tc.code || stderr.Len() != 0 || strings.Contains(stdout.String(), "模擬秘密値") {
					t.Fatalf("CLIの終了コードと出力が一致しません: %d %q %q", code, stdout.String(), stderr.String())
				}
				if asJSON {
					result := decodeReport(t, stdout.String())
					if result.Status != tc.status || result.ExitCode() != tc.code || result.Counts != tc.counts {
						t.Fatalf("JSONの状態・件数・終了コードが一致しません: %+v", result)
					}
				} else if !strings.HasPrefix(stdout.String(), tc.text+"\n") || (tc.code == 0 && !strings.Contains(stdout.String(), fmt.Sprintf("%q", path))) {
					t.Fatalf("通常表示の状態または候補のパスが一致しません: %q", stdout.String())
				}
			}
		})
	}
}
