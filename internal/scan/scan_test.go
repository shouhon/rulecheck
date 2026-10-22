package scan_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"rulecheck/internal/report"
	"rulecheck/internal/scan"
)

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

func symlink(t *testing.T, target, path string) {
	t.Helper()
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("共有リンクを作成できないため未確認: %v", err)
	}
}

func hasCode(result report.Result, code string, category report.Category) bool {
	return slices.ContainsFunc(result.Findings, func(f report.Finding) bool { return f.Code == code && f.Category == category })
}

func TestRulesIdentityAndRelativePaths(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "実体 空白.md")
	write(t, real, "模擬秘密値は出力しない\n")
	other := filepath.Join(dir, "別の指示.md")
	write(t, other, "内容が違ってもエラーにしない\n")
	t.Chdir(dir)
	options := scan.Options{Rules: []string{real, "実体 空白.md", other, real}}
	result := scan.Run(options)
	if len(result.Results) != 2 || result.Counts != (report.Counts{}) || result.ExitCode() != 0 {
		t.Fatalf("同一実体の集約または分類が一致しません: %+v", result)
	}
	for _, target := range result.Results {
		if target.ResolvedPath == real && len(target.Aliases) != 2 {
			t.Fatalf("相対・絶対パスの別名が残っていません: %+v", target)
		}
	}
	first, _ := json.Marshal(result)
	slices.Reverse(options.Rules)
	second, _ := json.Marshal(scan.Run(options))
	if string(first) != string(second) || strings.Contains(string(first), "模擬秘密値") {
		t.Fatalf("並び順が不安定、または本文が出力されています: %s", first)
	}
}

func TestRuleLinks(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "実体 空白.md")
	write(t, real, "模擬本文\n")
	alias := filepath.Join(dir, "別名.md")
	symlink(t, filepath.Base(real), alias)
	chain := filepath.Join(dir, "連鎖.md")
	symlink(t, filepath.Base(alias), chain)
	result := scan.Run(scan.Options{Rules: []string{chain, real, alias}})
	if len(result.Results) != 1 || result.Counts != (report.Counts{}) {
		t.Fatalf("共有リンクの実体が集約されていません: %+v", result)
	}
	target := result.Results[0]
	if target.ResolvedPath != real || len(target.Aliases) != 3 || len(target.Links) != 2 {
		t.Fatalf("実体・別名・参照関係・記述元が一致しません: %+v", target)
	}
}

func TestBrokenAndCyclicLinks(t *testing.T) {
	dir := t.TempDir()
	broken := filepath.Join(dir, "切れたリンク.md")
	symlink(t, "不在.md", broken)
	a, b := filepath.Join(dir, "循環A.md"), filepath.Join(dir, "循環B.md")
	symlink(t, filepath.Base(b), a)
	symlink(t, filepath.Base(a), b)
	self := filepath.Join(dir, "自己循環.md")
	symlink(t, filepath.Base(self), self)
	for _, tc := range []struct{ path, code string }{{broken, "link_broken"}, {a, "link_cycle"}, {self, "link_cycle"}} {
		t.Run(tc.code+filepath.Base(tc.path), func(t *testing.T) {
			result := scan.Run(scan.Options{Rules: []string{tc.path}})
			if !hasCode(result.Results[0], tc.code, report.Error) || result.Counts.Error != 1 || result.ExitCode() != 1 {
				t.Fatalf("リンクの失敗が区別されていません: %+v", result)
			}
		})
	}
}

func TestHardLinkIdentity(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.md"), filepath.Join(dir, "b.md")
	write(t, a, "模擬本文\n")
	if err := os.Link(a, b); err != nil {
		t.Skipf("ハードリンクを作成できないため未確認: %v", err)
	}
	result := scan.Run(scan.Options{Rules: []string{b, a}})
	if len(result.Results) != 1 || len(result.Results[0].Aliases) != 2 || result.Results[0].ResolvedPath != a {
		t.Fatalf("同一実体のハードリンクが集約されていません: %+v", result)
	}
}

func TestMissingFileBelowValidDirectoryLink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "実体")
	if err := os.Mkdir(real, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(dir, "別名")
	symlink(t, real, alias)
	result := scan.Run(scan.Options{Rules: []string{filepath.Join(alias, "不在.md")}})
	if !hasCode(result.Results[0], "path_missing", report.Error) || hasCode(result.Results[0], "link_broken", report.Error) {
		t.Fatalf("正常な親リンクをリンク切れと誤認しています: %+v", result)
	}
}

func TestPathAndTextFailures(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "binary.md")
	write(t, binary, "模擬秘密値\x00\xff")
	file := filepath.Join(dir, "file.md")
	write(t, file, "模擬本文\n")
	for _, tc := range []struct {
		name, path, code string
		skills           bool
		category         report.Category
	}{
		{"不在", filepath.Join(dir, "missing.md"), "path_missing", false, report.Error},
		{"指示にディレクトリ", dir, "path_not_file", false, report.Error},
		{"親にファイル", file, "path_not_directory", true, report.Error},
		{"テキスト以外", binary, "text_unsupported", false, report.Unknown},
		{"ネットワーク", `\\invalid.example\share\rules.md`, "path_not_local", false, report.Unknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			options := scan.Options{Rules: []string{tc.path}}
			if tc.skills {
				options = scan.Options{Skills: []string{tc.path}}
			}
			result := scan.Run(options)
			wantExit := 1
			if tc.category == report.Unknown {
				wantExit = 2
			}
			if !hasCode(result.Results[0], tc.code, tc.category) || result.ExitCode() != wantExit {
				t.Fatalf("種類・不存在・判定不能が区別されていません: %+v", result)
			}
			data, _ := json.Marshal(result)
			if strings.Contains(string(data), "模擬秘密値") {
				t.Fatal("本文が出力に漏れています")
			}
		})
	}
}

func TestDirectSkillsOnly(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "直下スキル", "SKILL.md")
	write(t, file, "---\nname: 直下スキル\ndescription: 説明\n---\n模擬本文\n")
	write(t, filepath.Join(dir, "補助", "事例", "SKILL.md"), "入れ子は対象外\n")
	write(t, filepath.Join(dir, "SKILL.md"), "親自身のファイルは対象外\n")
	write(t, filepath.Join(dir, "通常ファイル"), "ディレクトリ以外は対象外\n")
	result := scan.Run(scan.Options{Skills: []string{dir, dir}})
	if len(result.Results) != 2 || result.Counts != (report.Counts{}) || result.Coverage.Skills != report.Completed {
		t.Fatalf("直下以外を走査したか、スキル検査が完了していません: %+v", result)
	}
	for _, target := range result.Results {
		if target.Kind == "skill" && target.ResolvedPath != file {
			t.Fatalf("直下以外のスキルが含まれています: %+v", target)
		}
	}
}

func TestSkillDirectoryAliases(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "skills")
	file := filepath.Join(root, "実体", "SKILL.md")
	write(t, file, "模擬本文\n")
	alias := filepath.Join(dir, "親の別名")
	symlink(t, root, alias)
	symlink(t, "実体", filepath.Join(root, "スキルの別名"))
	result := scan.Run(scan.Options{Skills: []string{root, alias}})
	if len(result.Results) != 2 {
		t.Fatalf("親またはスキルの同一実体が集約されていません: %+v", result)
	}
	for _, target := range result.Results {
		if target.Kind == "skill" && (target.ResolvedPath != file || len(target.Aliases) != 4 || len(target.Links) != 2) {
			t.Fatalf("スキルの別名とリンクの対応が失われています: %+v", target)
		}
	}
}

func TestBrokenSkillFileLink(t *testing.T) {
	dir := t.TempDir()
	skill := filepath.Join(dir, "skill")
	if err := os.Mkdir(skill, 0700); err != nil {
		t.Fatal(err)
	}
	symlink(t, "不在.md", filepath.Join(skill, "SKILL.md"))
	result := scan.Run(scan.Options{Skills: []string{dir}})
	if len(result.Results) != 2 || !hasCode(result.Results[0], "link_broken", report.Error) {
		t.Fatalf("壊れたSKILL.mdを補助ディレクトリと誤認しています: %+v", result)
	}
}
