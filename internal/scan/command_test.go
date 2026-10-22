package scan_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"rulecheck/internal/report"
	"rulecheck/internal/scan"
)

func commandFixture(t *testing.T, dir, name string) string {
	t.Helper()
	body := "#!/bin/sh\nprintf '起動された' > started.txt\n# 模擬秘密値\n"
	if runtime.GOOS == "windows" {
		name += ".cmd"
		body = "@echo off\r\necho 起動された>started.txt\r\nrem 模擬秘密値\r\n"
	}
	path := filepath.Join(dir, name)
	write(t, path, body)
	if err := os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCommandsSearchWithoutExecution(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	first, second := filepath.Join(dir, "候補 空白"), filepath.Join(dir, "次の候補")
	a := commandFixture(t, first, "mock")
	commandFixture(t, second, "mock")
	t.Setenv("PATH", first+string(os.PathListSeparator)+second)
	t.Setenv("PATHEXT", ".CMD;.EXE")
	options := scan.Options{Commands: []string{"missing", "mock", "mock"}}
	result := scan.Run(options)
	if result.ExitCode() != 1 || result.Counts != (report.Counts{Error: 1}) || len(result.Results) != 2 || result.Coverage.Commands != report.Completed {
		t.Fatalf("CLIの候補と不足が区別されていません: %+v", result)
	}
	if !hasCode(result.Results[0], "command_missing", report.Error) || result.Results[1].ResolvedPath != a || len(result.Results[1].Findings) != 0 {
		t.Fatalf("PATHの順序で最初の候補が選ばれていません: %+v", result.Results)
	}
	if _, err := os.Stat(filepath.Join(dir, "started.txt")); !os.IsNotExist(err) {
		t.Fatalf("候補が起動されたか、起動痕跡を確認できません: %v", err)
	}
	before, _ := json.Marshal(result)
	slices.Reverse(options.Commands)
	after, _ := json.Marshal(scan.Run(options))
	if string(before) != string(after) || strings.Contains(string(before), "模擬秘密値") {
		t.Fatal("指定順で出力が変わったか、候補の本文が出力されています")
	}
}

func TestCommandsOnlySearchPATH(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	file := commandFixture(t, dir, "mock")
	t.Setenv("PATHEXT", ".CMD")
	for _, entry := range []string{"", filepath.Join(dir, "不在"), file} {
		t.Setenv("PATH", entry)
		result := scan.Run(scan.Options{Commands: []string{"mock"}})
		if result.ExitCode() != 1 || !hasCode(result.Results[0], "command_missing", report.Error) {
			t.Fatalf("PATH以外の作業ディレクトリを暗黙に検索しています: %+v", result)
		}
	}
	// PATHに明記した相対ディレクトリは、作業ディレクトリから解決する。
	t.Setenv("PATH", ".")
	result := scan.Run(scan.Options{Commands: []string{"mock"}})
	if result.ExitCode() != 0 || result.Results[0].ResolvedPath != file {
		t.Fatalf("明示した相対PATHの候補が見つかりません: %+v", result)
	}
}

func TestCommandsUnresolvedPATH(t *testing.T) {
	dir := t.TempDir()
	file := commandFixture(t, dir, "mock")
	t.Setenv("PATHEXT", ".CMD")
	t.Setenv("PATH", `\\invalid.example\share`+string(os.PathListSeparator)+dir)
	result := scan.Run(scan.Options{Commands: []string{"mock", "missing"}})
	if result.ExitCode() != 2 || result.Counts != (report.Counts{Unknown: 1}) || result.Coverage.Commands != report.Partial {
		t.Fatalf("開かないPATHをCLI不足と誤認しています: %+v", result)
	}
	if !hasCode(result.Results[0], "command_unresolved", report.Unknown) || result.Results[1].ResolvedPath != file {
		t.Fatalf("別のローカルPATHで確認できた候補が残っていません: %+v", result)
	}
}

func TestCommandLinksAndDirectories(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	real := commandFixture(t, filepath.Join(dir, "実体"), "mock")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(bin, filepath.Base(real))
	symlink(t, real, alias)
	t.Setenv("PATH", bin)
	t.Setenv("PATHEXT", ".CMD")
	result := scan.Run(scan.Options{Commands: []string{"mock"}})
	if result.ExitCode() != 0 || result.Results[0].ResolvedPath != real || len(result.Results[0].Links) != 1 {
		t.Fatalf("CLIの共有リンクが解決されていません: %+v", result)
	}
	name := "directory"
	if runtime.GOOS == "windows" {
		name += ".cmd"
	}
	if err := os.Mkdir(filepath.Join(bin, name), 0700); err != nil {
		t.Fatal(err)
	}
	result = scan.Run(scan.Options{Commands: []string{"directory"}})
	if result.ExitCode() != 1 || !hasCode(result.Results[0], "command_missing", report.Error) {
		t.Fatalf("ディレクトリを実行ファイル候補と誤認しています: %+v", result)
	}
}
