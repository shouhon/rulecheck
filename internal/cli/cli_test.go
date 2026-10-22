package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/shouhon/rulecheck/internal/cli"
	"github.com/shouhon/rulecheck/internal/version"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{name: "使い方", args: []string{"--help"}, wantStdout: "使い方:"},
		{name: "使い方の短縮形", args: []string{"-h"}, wantStdout: "使い方:"},
		{name: "検査の使い方", args: []string{"scan", "--help"}, wantStdout: "ローカル参照"},
		{name: "版番号", args: []string{"--version"}, wantStdout: "rulecheck " + version.Current + "\n"},
		{name: "引数なし", wantCode: 64, wantStderr: "使い方:"},
		{name: "不明なコマンド", args: []string{"unknown"}, wantCode: 64, wantStderr: "引数が正しくありません"},
		{name: "使い方の余分な引数", args: []string{"--help", "extra"}, wantCode: 64, wantStderr: "引数が正しくありません"},
		{name: "版番号の余分な引数", args: []string{"--version", "extra"}, wantCode: 64, wantStderr: "引数が正しくありません"},
		{name: "対象なし", args: []string{"scan"}, wantCode: 64, wantStderr: "引数が正しくありません"},
		{name: "不存在を検査成功にしない", args: []string{"scan", "--rules", "存在しない指示.md"}, wantCode: 1, wantStdout: "エラーあり"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := cli.Run(tt.args, &stdout, &stderr)
			if code != tt.wantCode {
				t.Fatalf("終了コード = %d, 期待値 = %d", code, tt.wantCode)
			}
			for _, output := range []struct {
				name string
				got  string
				want string
			}{
				{name: "標準出力", got: stdout.String(), want: tt.wantStdout},
				{name: "標準エラー", got: stderr.String(), want: tt.wantStderr},
			} {
				if output.want == "" && output.got != "" {
					t.Errorf("%sに不要な出力があります: %q", output.name, output.got)
				} else if output.want != "" && !strings.Contains(output.got, output.want) {
					t.Errorf("%s = %q, 含むべき内容 = %q", output.name, output.got, output.want)
				}
			}
		})
	}
}
