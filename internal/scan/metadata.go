package scan

import (
	"bytes"
	"io"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/shouhon/rulecheck/internal/report"
)

// checkMetadata は値を診断へ転載せず、Markdownの解析から先頭メタデータを除く。
func checkMetadata(t *target, body []byte) []byte {
	lines := bytes.SplitAfter(body, []byte("\n"))
	if strings.TrimRight(string(lines[0]), " \t\r\n") != "---" {
		t.result.Findings = append(t.result.Findings, report.Finding{
			Code: "metadata_missing", Category: report.Error, Line: 1,
			Message: "先頭のYAMLメタデータがありません。",
		})
		return body
	}
	end, offset := -1, len(lines[0])
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(string(lines[i]), " \t\r\n") == "---" {
			end = i
			break
		}
		offset += len(lines[i])
	}
	if end < 0 {
		t.result.Findings = append(t.result.Findings,
			report.Finding{Code: "metadata_invalid", Category: report.Error, Line: 1, Message: "YAMLメタデータの終端がありません。"},
			report.Finding{Code: "reference_unresolved", Category: report.Unknown, Line: 1, Message: "本文の開始位置が不明なため、Markdown参照を検査できません。"},
		)
		return nil
	}
	payload := body[len(lines[0]):offset]
	// バイト位置と行番号を保ち、値に含まれるリンク例は参照として解析しない。
	markdown := bytes.Clone(body)
	for i := range offset + len(lines[end]) {
		if markdown[i] != '\n' && markdown[i] != '\r' {
			markdown[i] = ' '
		}
	}
	decoder := yaml.NewDecoder(bytes.NewReader(payload))
	var doc, extra yaml.Node
	err := decoder.Decode(&doc)
	valid := err == nil && len(doc.Content) == 1 && doc.Content[0].Kind == yaml.MappingNode && doc.Content[0].Tag == "!!map"
	if valid {
		// 構文木の取得だけでは検出しない重複キーと不正な別名参照も確認する。
		var values map[string]any
		valid = doc.Decode(&values) == nil && decoder.Decode(&extra) == io.EOF
	}
	if !valid {
		t.result.Findings = append(t.result.Findings, report.Finding{
			Code: "metadata_invalid", Category: report.Error, Line: 2,
			Message: "YAMLメタデータを項目の対応表として解釈できません。",
		})
		return markdown
	}
	root := doc.Content[0]
	for _, key := range []string{"name", "description"} {
		value, line := metadataValue(root, key)
		if value == nil {
			t.result.Findings = append(t.result.Findings, report.Finding{
				Code: "metadata_" + key + "_missing", Category: report.Error, Line: 2,
				Message: "必須項目 " + key + " がありません。",
			})
			continue
		}
		if value.Kind != yaml.ScalarNode || value.Tag != "!!str" || strings.TrimSpace(value.Value) == "" {
			t.result.Findings = append(t.result.Findings, report.Finding{
				Code: "metadata_" + key + "_invalid", Category: report.Error, Line: line + 1,
				Message: "必須項目 " + key + " は空ではない文字列で指定してください。",
			})
			continue
		}
		if key == "name" {
			t.name, t.nameLine = value.Value, line+1
		}
	}
	return markdown
}

// metadataValue は通常の項目を優先し、YAMLの別名とマージも型を保ってたどる。
// 循環する別名は事前のDecodeで拒否されている。
func metadataValue(node *yaml.Node, key string) (*yaml.Node, int) {
	if node.Kind == yaml.AliasNode {
		return metadataValue(node.Alias, key)
	}
	if node.Kind == yaml.SequenceNode {
		for _, child := range node.Content {
			if value, line := metadataValue(child, key); value != nil {
				return value, line
			}
		}
	}
	if node.Kind != yaml.MappingNode {
		return nil, 0
	}
	var merge *yaml.Node
	for i := 0; i < len(node.Content); i += 2 {
		k, v := node.Content[i], node.Content[i+1]
		if k.Tag == "!!str" && k.Value == key {
			line := k.Line
			for v.Kind == yaml.AliasNode {
				v = v.Alias
			}
			return v, line
		}
		if k.Tag == "!!merge" {
			merge = v
		}
	}
	if merge != nil {
		return metadataValue(merge, key)
	}
	return nil, 0
}

func (s *scanner) checkSkillNames() {
	groups := make(map[string][]*target)
	for _, t := range s.targets {
		if t.result.Kind != "skill" || t.name == "" {
			continue
		}
		groups[t.name] = append(groups[t.name], t)
		if filepath.Base(filepath.Dir(t.result.ResolvedPath)) != t.name {
			t.result.Findings = append(t.result.Findings, report.Finding{
				Code: "skill_name_mismatch", Category: report.Warning, Line: t.nameLine,
				Message: "スキルの実体ディレクトリ名とnameが異なります。",
			})
		}
	}
	for _, group := range groups {
		if len(group) < 2 {
			continue
		}
		paths := make([]string, 0, len(group))
		for _, t := range group {
			paths = append(paths, t.result.ResolvedPath)
		}
		for _, t := range group {
			t.result.Findings = append(t.result.Findings, report.Finding{
				Code: "skill_name_duplicate", Category: report.Warning, Line: t.nameLine,
				Message: "異なる実体のスキルでnameが重複しています。", Paths: append([]string{}, paths...),
			})
		}
	}
}
