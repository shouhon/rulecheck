// Package scan は明示されたファイルと直下のスキルを読み取り専用で検査する。
package scan

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"unicode/utf8"

	"rulecheck/internal/report"
)

type Options struct {
	Rules    []string
	Skills   []string
	Commands []string
}

type target struct {
	result report.Result
	info   fs.FileInfo
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
		coverage.Rules, coverage.References = report.Completed, report.NotImplemented
		for _, path := range sortedUnique(options.Rules) {
			t, fresh := s.add("rules", path, false)
			if fresh {
				if t.info != nil {
					checkText(t)
				}
				t.result.Findings = append(t.result.Findings, report.Finding{
					Code: "rules_not_implemented", Category: report.Unknown,
					Message: "Markdownの参照検査は未実装です。",
				})
			}
		}
	}
	if len(options.Skills) != 0 {
		coverage.Skills, coverage.References = report.Partial, report.NotImplemented
		var roots []*target
		for _, path := range sortedUnique(options.Skills) {
			t, fresh := s.add("skills", path, true)
			if fresh {
				roots = append(roots, t)
				t.result.Findings = append(t.result.Findings, report.Finding{
					Code: "skills_not_implemented", Category: report.Unknown,
					Message: "スキルのメタデータとMarkdownの参照検査は未実装です。",
				})
			}
		}
		for _, root := range roots {
			if root.info != nil {
				s.skills(root)
			}
		}
	}
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
				f.Paths = append([]string{}, t.result.Aliases...)
				if t.result.ResolvedPath != "" && !slices.Contains(f.Paths, t.result.ResolvedPath) {
					f.Paths = append(f.Paths, t.result.ResolvedPath)
				}
				if t.result.Kind == "rules" || t.result.Kind == "skill" {
					f.Source = t.result.ResolvedPath
					if f.Source == "" {
						f.Source = t.result.Target
					}
				}
			}
			if f.Category == report.Unknown && f.Code != "rules_not_implemented" && t.result.Kind == "rules" {
				coverage.Rules = report.Partial
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
		t.result.Findings = append(t.result.Findings, report.Finding{Code: "path_not_directory", Category: report.Error, Message: "スキルの親にディレクトリ以外が指定されています。"})
	} else if !directory && !r.info.Mode().IsRegular() {
		t.result.Findings = append(t.result.Findings, report.Finding{Code: "path_not_file", Category: report.Error, Message: "対象は通常のファイルではありません。"})
	} else {
		t.info = r.info
	}
	return t, true
}

func checkText(t *target) {
	body, err := os.ReadFile(t.result.ResolvedPath)
	if err != nil {
		t.result.Findings = append(t.result.Findings, report.Finding{Code: "path_unreadable", Category: report.Unknown, Message: "対象の本文を読み取れません。"})
	} else if !utf8.Valid(body) || bytes.IndexByte(body, 0) >= 0 {
		t.result.Findings = append(t.result.Findings, report.Finding{Code: "text_unsupported", Category: report.Unknown, Message: "UTF-8の通常テキストとして本文を解釈できません。"})
	}
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
		if errors.Is(err, fs.ErrNotExist) && len(checked.links) == 0 {
			// 補助ディレクトリや入れ子のスキルは対象にしない。
			continue
		}
		for _, alias := range root.result.Aliases {
			path := filepath.Join(alias, entry.Name(), "SKILL.md")
			t, fresh := s.add("skill", path, false)
			if fresh && t.info != nil {
				checkText(t)
			}
		}
	}
}
