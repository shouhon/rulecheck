package scan_test

import (
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"

	"rulecheck/internal/report"
	"rulecheck/internal/scan"
)

func TestWindowsUnreadableFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "読取不能.md")
	write(t, path, "模擬本文\n")
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := syscall.CreateFile(name, syscall.GENERIC_READ, 0, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.CloseHandle(handle)
	result := scan.Run(scan.Options{Rules: []string{path}})
	if !hasCode(result.Results[0], "path_unreadable", report.Unknown) || result.Coverage.Rules != report.Partial || result.Counts.Error != 0 {
		t.Fatalf("読取不能を不存在や成功と誤認しています: %+v", result)
	}
	alias := filepath.Join(filepath.Dir(path), "別名.md")
	symlink(t, filepath.Base(path), alias)
	linked := scan.Run(scan.Options{Rules: []string{alias}})
	if !hasCode(linked.Results[0], "path_unreadable", report.Unknown) || hasCode(linked.Results[0], "link_broken", report.Error) {
		t.Fatalf("リンク先の読取不能が区別されていません: %+v", linked)
	}
}

func TestWindowsSpecialPaths(t *testing.T) {
	// 拡張子付きのCON.mdなどはWindows 11で通常名として扱うため含めない。
	for _, path := range []string{`NUL`, `CON`, `\\.\NUL`, `\\?\UNC\invalid.example\share\file.md`, `file.md:stream`} {
		t.Run(path, func(t *testing.T) {
			result := scan.Run(scan.Options{Rules: []string{path}})
			if !hasCode(result.Results[0], "path_not_local", report.Unknown) {
				t.Fatalf("特殊なパスを開こうとしています: %+v", result)
			}
		})
	}
}

func TestWindowsExtendedDrivePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "日本語 空白.md")
	write(t, path, "模擬本文\n")
	result := scan.Run(scan.Options{Rules: []string{`\\?\` + path, path}})
	if len(result.Results) != 1 || result.Results[0].ResolvedPath != path || len(result.Results[0].Aliases) != 2 {
		t.Fatalf("拡張長のドライブパスを解決できません: %+v", result)
	}
}

func TestWindowsRemoteLink(t *testing.T) {
	path := filepath.Join(t.TempDir(), "外部.md")
	name, _ := syscall.UTF16PtrFromString(path)
	target, _ := syscall.UTF16PtrFromString(`\\invalid.example\share\rules.md`)
	// os.Symlinkによる参照先の種類確認を避け、模擬のUNCへ接続しない。
	if err := syscall.CreateSymbolicLink(name, target, 2); err != nil {
		t.Skipf("共有リンクを作成できないため未確認: %v", err)
	}
	result := scan.Run(scan.Options{Rules: []string{path}})
	if !hasCode(result.Results[0], "path_not_local", report.Unknown) || len(result.Results[0].Links) != 1 {
		t.Fatalf("ネットワークのリンク先を避けられていません: %+v", result)
	}
}

func TestWindowsJunction(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "実体")
	file := filepath.Join(root, "skill", "SKILL.md")
	write(t, file, "模擬本文\n")
	alias := filepath.Join(dir, "別名")
	if output, err := exec.Command("cmd", "/c", "mklink", "/J", alias, root).CombinedOutput(); err != nil {
		t.Skipf("ジャンクションを作成できないため未確認: %v %s", err, output)
	}
	result := scan.Run(scan.Options{Skills: []string{root, alias}})
	if len(result.Results) != 2 {
		t.Fatalf("ジャンクションの実体を集約できません: %+v", result)
	}
	for _, target := range result.Results {
		if len(target.Aliases) != 2 || len(target.Links) != 1 {
			t.Fatalf("ジャンクションの別名と参照関係がありません: %+v", target)
		}
	}
}
