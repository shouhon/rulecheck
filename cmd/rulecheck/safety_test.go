package main_test

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestReadOnlyDependencies(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	// テスト自身の通信・プロセス起動は含めず、実行ファイルの依存だけを調べる。
	command := exec.CommandContext(ctx, "go", "list", "-deps", "-f", "{{.ImportPath}}", ".")
	output, err := command.Output()
	if err != nil {
		t.Fatalf("実行ファイルの依存を確認できません: %v", err)
	}
	for _, path := range strings.Fields(string(output)) {
		switch path {
		case "net":
			t.Error("実行ファイルの依存に通信機能のnetが含まれています")
		case "os/exec":
			t.Error("実行ファイルの依存にプロセス起動機能のos/execが含まれています")
		}
	}
}
