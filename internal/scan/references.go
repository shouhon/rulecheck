package scan

import (
	"bytes"
	"net/url"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"github.com/shouhon/rulecheck/internal/report"
)

var uriScheme = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)
var unsupportedLink = regexp.MustCompile(`\[\[|\[[^\]\n]*\]\s*[\[(]`)
var htmlReference = regexp.MustCompile(`(?i)\b(?:href|src|srcset|action|formaction|poster|data|background|cite|longdesc|usemap|profile|manifest|ping|archive|codebase|classid)\s*=|\burl\s*\(`)
var htmlComment = regexp.MustCompile(`(?s)<!--.*?(?:-->|$)`)

func checkReferences(t *target, body []byte, counts *report.ReferenceCounts) {
	record := func(finding report.Finding, passed bool) {
		if passed {
			counts.Checked++
			counts.Passed++
		} else if finding.Category == report.Error {
			counts.Checked++
			counts.Error++
		} else if finding.Category == report.Unknown {
			counts.Checked++
			counts.Unknown++
		}
		if finding.Category == report.Skipped {
			counts.Skipped++
		}
		if finding.Code != "" {
			t.result.Findings = append(t.result.Findings, finding)
		}
	}
	doc := goldmark.New().Parser().Parse(text.NewReader(body))
	_ = ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		line := sourceLine(body, node.Pos())
		if node.Type() == ast.TypeBlock && node.FirstChild() != nil && node.FirstChild().Type() == ast.TypeInline {
			if pos := unsupportedReference(node, body); pos >= 0 {
				record(report.Finding{
					Code: "reference_unsupported", Category: report.Unknown, Line: sourceLine(body, pos),
					Message: "リンクに似た記述を対応するMarkdown参照として解釈できません。", Paths: []string{},
				}, false)
			}
		}
		var finding report.Finding
		var passed bool
		switch n := node.(type) {
		case *ast.Link:
			finding, passed = checkReference(t.result.ResolvedPath, n.Destination)
		case *ast.Image:
			finding, passed = checkReference(t.result.ResolvedPath, n.Destination)
		case *ast.AutoLink:
			// Windowsのドライブ表記も自動リンクになるため、同じ境界で判定する。
			finding, passed = checkReference(t.result.ResolvedPath, n.URL(body))
		case *ast.CodeSpan, *ast.CodeBlock, *ast.FencedCodeBlock:
			finding = report.Finding{Code: "reference_code_skipped", Category: report.Skipped, Message: "コード中の例は参照として検査しません。", Paths: []string{}}
		case *ast.RawHTML:
			finding = checkHTMLReferences(n.Segments.Value(body))
		case *ast.HTMLBlock:
			finding = checkHTMLReferences(n.Lines().Value(body))
		}
		finding.Line = line
		record(finding, passed)
		if finding.Code != "" {
			if node.Kind() == ast.KindCodeSpan || node.Kind() == ast.KindCodeBlock || node.Kind() == ast.KindFencedCodeBlock || node.Kind() == ast.KindHTMLBlock {
				return ast.WalkSkipChildren, nil
			}
		}
		return ast.WalkContinue, nil
	})
}

// unsupportedReference は装飾をまたぐ本文を確認する。成立済みリンクとコードの
// 中身は空白へ置換し、その前後に残った未解決の括弧だけを検出する。
func unsupportedReference(root ast.Node, body []byte) int {
	var value []byte
	var positions []int
	appendText := func(text []byte, start int) {
		value = append(value, text...)
		for i := range text {
			positions = append(positions, start+i)
		}
	}
	_ = ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := node.(type) {
		case *ast.Link, *ast.Image, *ast.AutoLink, *ast.CodeSpan, *ast.RawHTML:
			appendText([]byte(" "), node.Pos())
			return ast.WalkSkipChildren, nil
		case *ast.Text:
			appendText(n.Segment.Value(body), n.Segment.Start)
			if n.SoftLineBreak() || n.HardLineBreak() {
				appendText([]byte("\n"), n.Segment.Stop)
			}
		case *ast.String:
			appendText(n.Value, n.Pos())
		}
		return ast.WalkContinue, nil
	})
	for _, match := range unsupportedLink.FindAllIndex(value, -1) {
		escapes := 0
		for i := match[0] - 1; i >= 0 && value[i] == '\\'; i-- {
			escapes++
		}
		if escapes%2 == 0 {
			return positions[match[0]]
		}
	}
	return -1
}

func checkHTMLReferences(value []byte) report.Finding {
	if htmlReference.Match(htmlComment.ReplaceAll(value, nil)) {
		return report.Finding{Code: "reference_unsupported", Category: report.Unknown, Message: "HTML内の参照は未対応です。", Paths: []string{}}
	}
	return report.Finding{}
}

func sourceLine(body []byte, pos int) int {
	if pos < 0 || pos > len(body) {
		return 0
	}
	return bytes.Count(body[:pos], []byte("\n")) + 1
}

// checkReference は診断と、参照先の存在・種類を確認できたかを返す。
func checkReference(source string, destination []byte) (report.Finding, bool) {
	f := report.Finding{Code: "reference_unsupported", Category: report.Unknown, Message: "参照先の表記を解釈できません。", Paths: []string{}}
	if bytes.HasPrefix(destination, []byte(`\\`)) {
		f.Message = "ネットワーク上または特殊な参照は検査できません。"
		return f, false
	}
	// Markdownのエスケープと文字参照を解き、URLのパーセント符号化は一度だけ解く。
	raw := string(util.ResolveEntityNames(util.ResolveNumericReferences(util.UnescapePunctuations(destination))))
	if strings.HasPrefix(raw, "#") || raw == "" {
		f.Code, f.Category, f.Message = "reference_fragment_skipped", report.Skipped, "同じ文書内の参照は検査しません。"
		return f, false
	}
	windowsPath := len(raw) >= 3 && isDriveLetter(raw[0]) && raw[1] == ':' && (raw[2] == '/' || raw[2] == '\\')
	if !windowsPath && uriScheme.MatchString(raw) {
		scheme, _, _ := strings.Cut(raw, ":")
		if strings.EqualFold(scheme, "file") || len(scheme) == 1 || strings.Contains(scheme, ".") && !strings.Contains(raw, "://") {
			f.Message = "file URI、ドライブ基準の相対参照、特殊なローカルパスと区別できない表記は未対応です。"
		} else {
			f.Code, f.Category, f.Message = "reference_external_skipped", report.Skipped, "外部の参照は検査しません。"
		}
		return f, false
	}
	path, _, fragment := strings.Cut(raw, "#")
	if strings.ContainsAny(path, "?\r\n") {
		f.Message = "問い合わせ文字列または改行を含むローカル参照は未対応です。"
		return f, false
	}
	path, err := url.PathUnescape(path)
	if err != nil || !utf8.ValidString(path) || strings.ContainsFunc(path, unicode.IsControl) || path == "" {
		return f, false
	}
	if windowsPath && runtime.GOOS != "windows" {
		f.Message = "Windowsの絶対パスはこの環境では確認できません。"
		return f, false
	}
	if strings.HasPrefix(path, `\\`) || strings.HasPrefix(path, "//") || filepath.VolumeName(path) != "" && !filepath.IsAbs(path) {
		f.Message = "ネットワーク上またはドライブ基準の参照は検査できません。"
		return f, false
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(filepath.Dir(source), path)
	}
	// 本文は開かず、既存のローカル専用解決で存在と種類だけを調べる。
	r, err := resolve(path)
	if err != nil {
		cause := pathFinding(err)
		f.Code, f.Category, f.Message = "reference_"+strings.TrimPrefix(cause.Code, "path_"), cause.Category, cause.Message
		// 診断にURL・問い合わせ・本文を転載しない。通常のローカルパスだけ残す。
		if cause.Code != "path_not_local" {
			f.Paths = []string{path}
		}
		return f, false
	}
	if !r.info.IsDir() && !r.info.Mode().IsRegular() {
		f.Code, f.Category, f.Message = "reference_not_regular", report.Unknown, "参照先は通常のファイルまたはディレクトリではありません。"
		f.Paths = []string{r.path}
		return f, false
	}
	if fragment {
		return report.Finding{Code: "reference_fragment_skipped", Category: report.Skipped, Message: "参照先の存在と種類を確認しました。見出し自体は検査しません。", Paths: []string{r.path}}, true
	}
	// 成功には診断を増やさない。種類は、異常な種類の判定と関係パスに反映する。
	return report.Finding{}, true
}

func isDriveLetter(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z'
}
