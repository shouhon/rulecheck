package scan_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shouhon/rulecheck/internal/report"
	"github.com/shouhon/rulecheck/internal/scan"
)

func TestMarkdownLocalReferences(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"通常.md", "空白 日本語.md", "括弧(a).md", "a&b.md", "hash#name.md"} {
		write(t, filepath.Join(dir, name), "[再帰しない](not-read.md)\n模擬秘密値\x00")
	}
	if err := os.Mkdir(filepath.Join(dir, "directory"), 0700); err != nil {
		t.Fatal(err)
	}
	body := "[通常](通常.md)\n[空白](<空白 日本語.md>)\n[符号化](空白%20日本語.md)\n" +
		"[括弧](括弧\\(a\\).md)\n[文字参照](a&amp;b.md)\n[hash](hash%23name.md)\n" +
		"[参照][定義]\n[短縮][]\n[省略]\n![画像][定義]\n![通常画像](通常.md)\n" +
		"[ディレクトリ](directory/)\n[見出し付き](通常.md#未検証)\n\n" +
		"[定義]: <空白 日本語.md> \"タイトル\"\n[短縮]: 通常.md\n[省略]:\n  通常.md\n"
	file := filepath.Join(dir, "rules.md")
	write(t, file, body)
	result := scan.Run(scan.Options{Rules: []string{file}})
	if result.ExitCode() != 0 || result.Counts != (report.Counts{Skipped: 1}) || result.Coverage.References != report.Completed {
		t.Fatalf("ローカル参照の構文または非再帰が一致しません: %+v", result)
	}
}

func TestMarkdownExcludedExamplesAndExternalURLs(t *testing.T) {
	file := filepath.Join(t.TempDir(), "rules.md")
	body := "`[例](missing.md)` と ``![例](missing.png)``\n\n" +
		"```md\n[例](missing.md)\n```\n\n~~~\n![例](missing.png)\n~~~\n\n" +
		"    [例](missing.md)\n\n> ```\n> [例](missing.md)\n> ```\n\n" +
		"[外部](https://invalid.example/模擬秘密値)\n[メール](mailto:模擬秘密値@example.invalid)\n" +
		"<https://invalid.example/模擬秘密値>\n[見出し](#heading)\n\\[文字](a b.md)\n"
	write(t, file, body)
	result := scan.Run(scan.Options{Rules: []string{file}})
	if result.ExitCode() != 0 || result.Counts.Error != 0 || result.Counts.Unknown != 0 || result.Counts.Skipped < 9 {
		t.Fatalf("コードや外部URLをローカル参照として扱っています: %+v", result)
	}
	data, _ := json.Marshal(result)
	if strings.Contains(string(data), "模擬秘密値") || strings.Contains(string(data), "missing.md") {
		t.Fatal("検査対象外の本文を出力しています")
	}
}

func TestMarkdownFailureLinesAndPartialCoverage(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "rules.md")
	write(t, file, "# 指示\n[壊れた参照](missing.md#heading)\n![画像](image.png)\n[外部](https://invalid.example/)\n")
	result := scan.Run(scan.Options{Rules: []string{file}})
	if result.ExitCode() != 1 || result.Counts != (report.Counts{Error: 2, Skipped: 1}) || result.Coverage.References != report.Completed {
		t.Fatalf("不存在の分類・件数が一致しません: %+v", result)
	}
	f := result.Results[0].Findings[0]
	if f.Code != "reference_missing" || f.Line != 2 || f.Source != file || len(f.Paths) != 1 || f.Paths[0] != filepath.Join(dir, "missing.md") {
		t.Fatalf("参照の記述位置と関係パスがありません: %+v", f)
	}
	for _, source := range []string{
		"[例](bad%zz.md)", "[例](file:///C:/secret.md)", "[例](C:relative.md)",
		"[例](a.md?secret)", "[[独自記法]]", "[未定義][label]", "[未*定義*][label]", "[未 `定義`][label]", "[空白](a b.md)",
		"<img src=\"missing.png\">", "<blockquote cite=\"missing.md\">本文</blockquote>", "<button formaction=\"missing\">本文</button>",
		"[例](%00.md)", "[例](//invalid.example/share)",
		"[例](%2f%2finvalid.example/share)", `[例](\\invalid.example\share)`,
	} {
		t.Run(source, func(t *testing.T) {
			write(t, file, source)
			result := scan.Run(scan.Options{Rules: []string{file}})
			if result.ExitCode() != 2 || result.Counts.Unknown == 0 || result.Coverage.Rules != report.Completed || result.Coverage.References != report.Partial {
				t.Fatalf("未対応の参照を検査完了として扱っています: %+v", result)
			}
		})
	}
}

func TestMarkdownHTMLWithoutReferences(t *testing.T) {
	file := filepath.Join(t.TempDir(), "rules.md")
	write(t, file, "改行<br>\n\n<!-- コメント内の例 [参照](missing.md) と <a href=\"missing\"> -->\n\n<strong>本文</strong>\n")
	result := scan.Run(scan.Options{Rules: []string{file}})
	if result.ExitCode() != 0 || result.Counts.Unknown != 0 || result.Counts.Error != 0 {
		t.Fatalf("参照を含まないHTMLを未対応の参照と扱っています: %+v", result)
	}
}

func TestMarkdownRelativeBaseUsesResolvedFile(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real", "rules.md")
	write(t, real, "[参照](<空白 日本語.md>)\n")
	write(t, filepath.Join(dir, "real", "空白 日本語.md"), "模擬秘密値")
	alias := filepath.Join(dir, "alias", "rules.md")
	if err := os.MkdirAll(filepath.Dir(alias), 0700); err != nil {
		t.Fatal(err)
	}
	symlink(t, real, alias)
	result := scan.Run(scan.Options{Rules: []string{alias}})
	if result.ExitCode() != 0 || result.Results[0].ResolvedPath != real {
		t.Fatalf("別名のディレクトリを相対参照の基準にしています: %+v", result)
	}
}

func TestMarkdownBrokenAndCyclicReferenceLinks(t *testing.T) {
	dir := t.TempDir()
	symlink(t, "absent.md", filepath.Join(dir, "broken.md"))
	symlink(t, "cycle.md", filepath.Join(dir, "cycle.md"))
	file := filepath.Join(dir, "rules.md")
	write(t, file, "[リンク切れ](broken.md)\n[循環](cycle.md)\n")
	result := scan.Run(scan.Options{Rules: []string{file}})
	if result.ExitCode() != 1 || !hasCode(result.Results[0], "reference_link_broken", report.Error) || !hasCode(result.Results[0], "reference_link_cycle", report.Error) {
		t.Fatalf("参照先のリンク切れと循環を区別できません: %+v", result)
	}
}

func TestSkillMarkdownStartsAfterMetadata(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "skill", "SKILL.md"), "---\nname: skill\ndescription: '[例](ignored.md)'\n---\n[参照](missing.md)\n")
	result := scan.Run(scan.Options{Skills: []string{dir}})
	f := result.Results[0].Findings[0]
	if result.Counts != (report.Counts{Error: 1}) || f.Code != "reference_missing" || f.Line != 5 || result.Coverage.Skills != report.Completed {
		t.Fatalf("メタデータのリンク除外または本文行番号が一致しません: %+v", result)
	}
}
