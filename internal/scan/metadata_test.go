package scan_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shouhon/rulecheck/internal/report"
	"github.com/shouhon/rulecheck/internal/scan"
)

func TestMetadataTypesAndSyntax(t *testing.T) {
	for _, tc := range []struct{ name, metadata, code string }{
		{"先頭なし", "本文だけ", "metadata_missing"},
		{"終端なし", "---\nname: skill\n", "metadata_invalid"},
		{"構文不正", "---\nname: [\ndescription: 模擬秘密値\n---\n", "metadata_invalid"},
		{"空", "---\n---\n", "metadata_invalid"},
		{"配列", "---\n- name\n---\n", "metadata_invalid"},
		{"複数文書", "---\nname: skill\n...\nname: other\n---\n", "metadata_invalid"},
		{"キー重複", "---\nname: skill\nname: other\ndescription: 模擬秘密値\n---\n", "metadata_invalid"},
		{"追加項目のキー重複", "---\nname: skill\ndescription: 説明\nextra: {a: 1, a: 2}\n---\n", "metadata_invalid"},
		{"別名循環", "---\nname: skill\ndescription: 説明\nextra: &cycle [*cycle]\n---\n", "metadata_invalid"},
		{"name欠落", "---\ndescription: 模擬秘密値\n---\n", "metadata_name_missing"},
		{"description欠落", "---\nname: skill\n---\n", "metadata_description_missing"},
		{"name整数", "---\nname: 123\ndescription: 説明\n---\n", "metadata_name_invalid"},
		{"name真偽値", "---\nname: true\ndescription: 説明\n---\n", "metadata_name_invalid"},
		{"name空白", "---\nname: '  '\ndescription: 説明\n---\n", "metadata_name_invalid"},
		{"description空", "---\nname: skill\ndescription: ''\n---\n", "metadata_description_invalid"},
		{"description整数", "---\nname: skill\ndescription: 123\n---\n", "metadata_description_invalid"},
		{"description真偽値", "---\nname: skill\ndescription: false\n---\n", "metadata_description_invalid"},
		{"description空値", "---\nname: skill\ndescription: null\n---\n", "metadata_description_invalid"},
		{"description配列", "---\nname: skill\ndescription: [説明]\n---\n", "metadata_description_invalid"},
		{"description対応表", "---\nname: skill\ndescription: {a: 説明}\n---\n", "metadata_description_invalid"},
		{"description日付", "---\nname: skill\ndescription: 2026-10-22\n---\n", "metadata_description_invalid"},
		{"descriptionバイナリ", "---\nname: skill\ndescription: !!binary c2VjcmV0\n---\n", "metadata_description_invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			write(t, filepath.Join(dir, "skill", "SKILL.md"), tc.metadata)
			result := scan.Run(scan.Options{Skills: []string{dir}})
			if !hasCode(result.Results[0], tc.code, report.Error) || result.Counts.Error == 0 || result.ExitCode() == 0 {
				t.Fatalf("メタデータの誤りを検出できません: %+v", result)
			}
			data, _ := json.Marshal(result)
			if strings.Contains(string(data), "模擬秘密値") || strings.Contains(string(data), "c2VjcmV0") {
				t.Fatal("YAMLの値を診断へ転載しています")
			}
		})
	}
}

func TestMetadataStringsAndAdditionalFields(t *testing.T) {
	for _, metadata := range []string{
		"name: skill\ndescription: 説明\nextra: {nested: [1, true, null]}\n",
		"name: 'skill'\ndescription: \"説明: # 続き\"\n",
		"name: skill\ndescription: >-\n  折り返す\n  説明\n",
		"name: skill\ndescription: |\n  複数行の\n  説明\n",
		"name: &name skill\ndescription: *name\n",
		"base: &base {name: skill, description: 説明}\n<<: *base\n",
		"base: &base {name: other, description: 説明}\n<<: *base\nname: skill\n",
	} {
		t.Run(metadata, func(t *testing.T) {
			dir := t.TempDir()
			// メタデータ中のリンクはMarkdownとして検査しない。
			body := "\xef\xbb\xbf---\n" + metadata + "secret: '[例](missing.md)'\n---\n本文\n"
			write(t, filepath.Join(dir, "skill", "SKILL.md"), strings.ReplaceAll(body, "\n", "\r\n"))
			result := scan.Run(scan.Options{Skills: []string{dir}})
			if result.ExitCode() != 0 || result.Counts != (report.Counts{}) || result.Coverage.Skills != report.Completed || result.Coverage.References != report.Completed {
				t.Fatalf("正しい文字列・追加項目を拒否しています: %+v", result)
			}
		})
	}
}

func TestSkillNamesAndIdentity(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "one"), filepath.Join(dir, "two")
	body := "---\nname: same\ndescription: 模擬秘密値\n---\n"
	write(t, filepath.Join(a, "same", "SKILL.md"), body)
	write(t, filepath.Join(b, "same", "SKILL.md"), body)
	symlink(t, filepath.Join(a, "same"), filepath.Join(a, "alias"))
	options := scan.Options{Skills: []string{b, a, a}}
	result := scan.Run(options)
	if result.ExitCode() != 0 || result.Status != "warning" || result.Counts != (report.Counts{Warning: 2}) {
		t.Fatalf("同じ実体と異なる実体の同名スキルを区別できません: %+v", result)
	}
	for _, target := range result.Results {
		if target.Kind == "skill" && (!hasCode(target, "skill_name_duplicate", report.Warning) || len(target.Findings[0].Paths) != 2 || target.Findings[0].Line != 2) {
			t.Fatalf("重複の関係パスと行番号がありません: %+v", target)
		}
	}
	first, _ := json.Marshal(result)
	second, _ := json.Marshal(scan.Run(scan.Options{Skills: []string{a, b}}))
	if string(first) != string(second) || strings.Contains(string(first), "模擬秘密値") {
		t.Fatal("スキル名の結果が不安定、または値が漏れています")
	}
}

func TestSkillDirectoryNameMismatch(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "directory", "SKILL.md"), "---\nname: 模擬秘密値\ndescription: 説明\n---\n")
	result := scan.Run(scan.Options{Skills: []string{dir}})
	if result.Counts != (report.Counts{Warning: 1}) || !hasCode(result.Results[0], "skill_name_mismatch", report.Warning) || result.ExitCode() != 0 {
		t.Fatalf("ディレクトリ名の差異が注意になっていません: %+v", result)
	}
	data, _ := json.Marshal(result)
	if strings.Contains(string(data), "模擬秘密値") {
		t.Fatal("スキル名の値が漏れています")
	}
}
