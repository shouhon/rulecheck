package scan_test

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/shouhon/rulecheck/internal/report"
	"github.com/shouhon/rulecheck/internal/scan"
)

func TestWindowsCommandExtensions(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"mock.exe", "mock.cmd", "explicit.custom", "plain", "script.ps1"} {
		write(t, filepath.Join(dir, name), "本文は実行しない\n")
	}
	t.Setenv("PATH", `"`+dir+`"`)
	for _, tc := range []struct{ name, extensions, want string }{
		{"mock", ".CMD;.EXE", "mock.cmd"},
		{"mock", "EXE;CMD", "mock.exe"},
		{"mock", "", "mock.exe"},
		{"mock.cmd", ".EXE", "mock.cmd"},
		{"explicit.custom", ".EXE", "explicit.custom"},
		{"plain", "", ""},
		{"script", "", ""},
	} {
		t.Run(tc.name+tc.extensions, func(t *testing.T) {
			t.Setenv("PATHEXT", tc.extensions)
			result := scan.Run(scan.Options{Commands: []string{tc.name}})
			if tc.want == "" {
				if result.ExitCode() != 1 || !hasCode(result.Results[0], "command_missing", report.Error) {
					t.Fatalf("実行対象外の拡張子を候補と誤認しています: %+v", result)
				}
			} else if result.ExitCode() != 0 || result.Results[0].ResolvedPath != filepath.Join(dir, tc.want) {
				t.Fatalf("PATHEXTと明示した拡張子が反映されていません: %+v", result)
			}
		})
	}
}

func TestWindowsCommandInvalidExtension(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	for _, extension := range []string{`.\..\outside.exe`, `:stream`, `*`, ".", ".exe "} {
		t.Setenv("PATHEXT", extension)
		result := scan.Run(scan.Options{Commands: []string{"mock"}})
		if result.ExitCode() != 2 || !hasCode(result.Results[0], "command_unresolved", report.Unknown) {
			t.Fatalf("不正な拡張子を通常の候補として扱っています: %+v", result)
		}
	}
}

func TestWindowsCommandDoesNotReadBody(t *testing.T) {
	dir := t.TempDir()
	path := commandFixture(t, dir, "mock")
	name, _ := syscall.UTF16PtrFromString(path)
	handle, err := syscall.CreateFile(name, syscall.GENERIC_READ, 0, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.CloseHandle(handle)
	t.Setenv("PATH", dir)
	t.Setenv("PATHEXT", ".CMD")
	result := scan.Run(scan.Options{Commands: []string{"mock"}})
	if result.ExitCode() != 0 || result.Results[0].ResolvedPath != path {
		t.Fatalf("本文を読めない候補の存在確認に失敗しました: %+v", result)
	}
}

func TestWindowsCommandRemoteLink(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mock.cmd")
	name, _ := syscall.UTF16PtrFromString(path)
	remote, _ := syscall.UTF16PtrFromString(`\\invalid.example\share\mock.cmd`)
	if err := syscall.CreateSymbolicLink(name, remote, 2); err != nil {
		t.Skipf("共有リンクを作成できないため未確認: %v", err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("PATHEXT", ".CMD")
	result := scan.Run(scan.Options{Commands: []string{"mock"}})
	if result.ExitCode() != 2 || !hasCode(result.Results[0], "command_unresolved", report.Unknown) {
		t.Fatalf("ネットワークのリンク先を通常の不在と誤認しています: %+v", result)
	}
	if _, err := os.Lstat(path); err != nil {
		t.Fatal("検査対象のリンクが変更されました")
	}
}
