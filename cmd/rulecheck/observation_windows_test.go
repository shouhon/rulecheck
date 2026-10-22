package main_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestObservationPreflight は準備の誤りを、受入試験の起動前に拒否することを確認する。
func TestObservationPreflight(t *testing.T) {
	script, err := filepath.Abs(filepath.Join("..", "..", "scripts", "run-observation.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	// 子プロセスが読むスクリプトも、Goのテストキャッシュの入力として記録する。
	if _, err := os.ReadFile(script); err != nil {
		t.Fatal(err)
	}
	type artifactHash struct {
		Path string
		Hash string
	}
	for _, tc := range []struct {
		name, diagnostic string
		wantSuccess      bool
		change           func(*testing.T, string, []artifactHash) []artifactHash
		args             []string
	}{
		{name: "事前確認では実行しない", diagnostic: "受入試験は実行していません", wantSuccess: true},
		{name: "成果物の改変", diagnostic: "準備済み成果物が変更されています", change: func(t *testing.T, dir string, hashes []artifactHash) []artifactHash {
			writeFile(t, filepath.Join(dir, "rulecheck.exe"), "変更後")
			return hashes
		}},
		{name: "ハッシュの重複", diagnostic: "hashes.jsonを読み取れないか形式が不正", change: func(_ *testing.T, _ string, hashes []artifactHash) []artifactHash {
			return []artifactHash{hashes[0], hashes[0]}
		}},
		{name: "別の準備先の混入", diagnostic: "hashes.jsonを読み取れないか形式が不正", change: func(_ *testing.T, dir string, hashes []artifactHash) []artifactHash {
			hashes[0].Path = filepath.Join(dir, "別の準備先", "rulecheck.exe")
			return hashes
		}},
		{name: "割当指定の誤り", diagnostic: "割当ドライブはY:;Z:のように指定", args: []string{"-NetworkDrives", `C:\folder`}},
		{name: "割当が見えない環境", diagnostic: "割当がこのPowerShellから見えません", args: []string{"-NetworkDrives", filepath.VolumeName(script)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			var hashes []artifactHash
			for _, name := range []string{"rulecheck.exe", "rulecheck-tests.exe"} {
				path := filepath.Join(dir, name)
				// 実行できない模擬ファイルにし、事前確認で起動しないことも確認する。
				body := "事前確認用の模擬成果物"
				writeFile(t, path, body)
				hashes = append(hashes, artifactHash{Path: path, Hash: fmt.Sprintf("%x", sha256.Sum256([]byte(body)))})
			}
			if tc.change != nil {
				hashes = tc.change(t, dir, hashes)
			}
			body, err := json.Marshal(hashes)
			if err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(dir, "hashes.json"), string(body))
			args := append([]string{"-NoProfile", "-File", script, "-ObservationDirectory", dir, "-CheckOnly"}, tc.args...)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			output, err := exec.CommandContext(ctx, "pwsh", args...).CombinedOutput()
			if (err == nil) != tc.wantSuccess || !strings.Contains(string(output), tc.diagnostic) {
				t.Fatalf("事前確認の結果が想定と異なります: %v\n%s", err, output)
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 3 {
				t.Fatal("事前確認中に試験の保存先や別のファイルを作成しました")
			}
		})
	}
}
