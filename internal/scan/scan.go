// Package scan は明示されたファイルと直下のスキルを読み取り専用で検査する。
package scan

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/shouhon/rulecheck/internal/report"
)

type Options struct {
	Rules     []string
	Skills    []string
	SkillDirs []string
	Commands  []string
}

type target struct {
	result   report.Result
	info     fs.FileInfo
	name     string
	nameLine int
}

type scanner struct {
	targets []*target
}

func Run(options Options) report.Report {
	s := &scanner{}
	coverage := report.Coverage{
		Rules: report.NotRequested, References: report.NotRequested,
		Skills: report.NotRequested, Commands: report.NotRequested,
	}
	if len(options.Rules) != 0 {
		coverage.Rules, coverage.References = report.Completed, report.Completed
		for _, path := range sortedUnique(options.Rules) {
			s.add("rules", path, false)
		}
	}
	if len(options.Skills) != 0 {
		coverage.Skills, coverage.References = report.Completed, report.Completed
		var roots []*target
		for _, path := range sortedUnique(options.Skills) {
			t, fresh := s.add("skills", path, true)
			if fresh {
				roots = append(roots, t)
			}
		}
		for _, root := range roots {
			if root.info != nil {
				s.skills(root)
			} else {
				coverage.References = report.Partial
			}
		}
	}
	if len(options.SkillDirs) != 0 {
		coverage.Skills = report.Completed
		if coverage.References == report.NotRequested {
			coverage.References = report.Completed
		}
		for _, path := range sortedUnique(options.SkillDirs) {
			dir, err := resolve(path)
			if err != nil || !dir.info.IsDir() {
				s.add("skill", path, true)
				continue
			}
			s.add("skill", filepath.Join(path, "SKILL.md"), false)
		}
	}
	// 別名と実体パスを確定してから本文を読み、相対参照の基準を固定する。
	for _, t := range s.targets {
		if t.result.Kind != "rules" && t.result.Kind != "skill" {
			continue
		}
		if t.info == nil {
			coverage.References = report.Partial
			continue
		}
		body, ok := checkText(t)
		if !ok {
			coverage.References = report.Partial
			continue
		}
		if t.result.Kind == "skill" {
			body = checkMetadata(t, body)
		}
		checkReferences(t, body)
	}
	s.checkSkillNames()
	if len(options.Commands) != 0 {
		coverage.Commands = report.Completed
		for _, name := range sortedUnique(options.Commands) {
			result := checkCommand(name)
			for _, finding := range result.Findings {
				if finding.Category == report.Unknown {
					coverage.Commands = report.Partial
				}
			}
			s.targets = append(s.targets, &target{result: result})
		}
	}
	var results []report.Result
	for _, t := range s.targets {
		for i := range t.result.Findings {
			f := &t.result.Findings[i]
			if t.result.Kind != "command" {
				if f.Paths == nil {
					f.Paths = append([]string{}, t.result.Aliases...)
					if t.result.ResolvedPath != "" && !slices.Contains(f.Paths, t.result.ResolvedPath) {
						f.Paths = append(f.Paths, t.result.ResolvedPath)
					}
				}
				if t.result.Kind == "rules" || t.result.Kind == "skill" {
					f.Source = t.result.ResolvedPath
					if f.Source == "" {
						f.Source = t.result.Target
					}
				}
			}
			if f.Category == report.Unknown {
				switch {
				case strings.HasPrefix(f.Code, "reference_"):
					coverage.References = report.Partial
				case t.result.Kind == "rules":
					coverage.Rules, coverage.References = report.Partial, report.Partial
				case t.result.Kind == "skill" || t.result.Kind == "skills":
					coverage.Skills = report.Partial
					if !strings.HasPrefix(f.Code, "metadata_") {
						coverage.References = report.Partial
					}
				}
			}
		}
		results = append(results, t.result)
	}
	return report.New(coverage, results)
}

func sortedUnique(paths []string) []string {
	paths = slices.Clone(paths)
	slices.Sort(paths)
	return slices.Compact(paths)
}

func (s *scanner) add(kind, path string, directory bool) (*target, bool) {
	r, err := resolve(path)
	if err == nil {
		for _, t := range s.targets {
			if t.result.Kind == kind && t.info != nil && os.SameFile(t.info, r.info) {
				t.result.Aliases = append(t.result.Aliases, path)
				t.result.Links = append(t.result.Links, r.links...)
				if path < t.result.Target {
					t.result.Target = path
				}
				if r.path < t.result.ResolvedPath {
					t.result.ResolvedPath = r.path
				}
				return t, false
			}
		}
	}
	t := &target{result: report.Result{Kind: kind, Target: path, Aliases: []string{path}, Links: r.links}}
	s.targets = append(s.targets, t)
	if err != nil {
		t.result.Findings = append(t.result.Findings, pathFinding(err))
		return t, true
	}
	t.result.ResolvedPath = r.path
	if directory && !r.info.IsDir() {
		t.result.Findings = append(t.result.Findings, report.Finding{Code: "path_not_directory", Category: report.Error, Message: "対象はディレクトリではありません。"})
	} else if !directory && !r.info.Mode().IsRegular() {
		t.result.Findings = append(t.result.Findings, report.Finding{Code: "path_not_file", Category: report.Error, Message: "対象は通常のファイルではありません。"})
	} else {
		t.info = r.info
	}
	return t, true
}

func checkText(t *target) ([]byte, bool) {
	body, err := os.ReadFile(t.result.ResolvedPath)
	if err != nil {
		t.result.Findings = append(t.result.Findings, report.Finding{Code: "path_unreadable", Category: report.Unknown, Message: "対象の本文を読み取れません。"})
	} else if !utf8.Valid(body) || bytes.IndexByte(body, 0) >= 0 {
		t.result.Findings = append(t.result.Findings, report.Finding{Code: "text_unsupported", Category: report.Unknown, Message: "UTF-8の通常テキストとして本文を解釈できません。"})
	} else {
		return bytes.TrimPrefix(body, []byte{0xef, 0xbb, 0xbf}), true
	}
	return nil, false
}

func (s *scanner) skills(root *target) {
	entries, err := os.ReadDir(root.result.ResolvedPath)
	if err != nil {
		root.result.Findings = append(root.result.Findings, report.Finding{Code: "directory_unreadable", Category: report.Unknown, Message: "スキルの親ディレクトリを列挙できません。"})
		return
	}
	for _, entry := range entries {
		child := filepath.Join(root.result.ResolvedPath, entry.Name())
		dir, err := resolve(child)
		if err != nil {
			// 壊れた共有ディレクトリは、中身を確認できなかった対象として残す。
			for _, alias := range root.result.Aliases {
				s.add("skill", filepath.Join(alias, entry.Name()), true)
			}
			continue
		}
		if !dir.info.IsDir() {
			continue
		}
		file := filepath.Join(dir.path, "SKILL.md")
		checked, err := resolve(file)
		if err != nil && pathFinding(err).Code == "path_missing" && len(checked.links) == 0 {
			// 補助ディレクトリや入れ子のスキルは対象にしない。
			continue
		}
		for _, alias := range root.result.Aliases {
			path := filepath.Join(alias, entry.Name(), "SKILL.md")
			s.add("skill", path, false)
		}
	}
}
