// Package report は検査結果の分類、安定した並び順と終了コードを定める。
package report

import (
	"cmp"
	"slices"
	"strings"

	"github.com/shouhon/rulecheck/internal/version"
)

// SchemaVersion はJSON形式の版番号の正本。CLIの版とは独立して管理する。
const SchemaVersion = 1

type Category string

const (
	Error   Category = "error"
	Warning Category = "warning"
	Unknown Category = "unknown"
	Skipped Category = "skipped"
)

const (
	NotRequested   = "not_requested"
	NotImplemented = "not_implemented"
	NotRun         = "not_run"
	Completed      = "completed"
	Partial        = "partial"
)

type Tool struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Coverage struct {
	Rules      string `json:"rules"`
	References string `json:"references"`
	Skills     string `json:"skills"`
	Commands   string `json:"commands"`
}

type Finding struct {
	Code     string   `json:"code"`
	Category Category `json:"category"`
	Message  string   `json:"message"`
	Source   string   `json:"source"`
	Line     int      `json:"line,omitempty"`
	Paths    []string `json:"paths"`
}

type Result struct {
	Kind         string    `json:"kind"`
	Target       string    `json:"target"`
	Aliases      []string  `json:"aliases,omitempty"`
	ResolvedPath string    `json:"resolved_path,omitempty"`
	Links        []Link    `json:"links,omitempty"`
	Findings     []Finding `json:"findings"`
}

// Link は実体の解決中に通ったリンクと、そこに記録された参照先を表す。
type Link struct {
	Path   string `json:"path"`
	Target string `json:"target"`
}

type Counts struct {
	Error   int `json:"error"`
	Warning int `json:"warning"`
	Unknown int `json:"unknown"`
	Skipped int `json:"skipped"`
}

// ReferenceCounts は参照の検査結果と、対象外の診断件数を表す。
// 見出し付きローカル参照は、パスの成功と見出しの対象外の両方に数える。
type ReferenceCounts struct {
	Checked int `json:"checked"`
	Passed  int `json:"passed"`
	Error   int `json:"error"`
	Unknown int `json:"unknown"`
	Skipped int `json:"skipped"`
}

// SkippedCount は対象外の診断を機械判定用のコード別に集計した件数を表す。
type SkippedCount struct {
	Code  string `json:"code"`
	Count int    `json:"count"`
}

type Report struct {
	SchemaVersion   int             `json:"schema_version"`
	Tool            Tool            `json:"tool"`
	Command         string          `json:"command"`
	Status          string          `json:"status"`
	Coverage        Coverage        `json:"coverage"`
	Results         []Result        `json:"results"`
	TargetCount     *int            `json:"target_count,omitempty"`
	Counts          Counts          `json:"counts"`
	ReferenceCounts ReferenceCounts `json:"reference_counts"`
	SkippedSummary  []SkippedCount  `json:"skipped_summary,omitempty"`
}

// New は結果と関係するパスを並べ替え、分類別件数から状態を決定する。
func New(coverage Coverage, results []Result) Report {
	if results == nil {
		results = []Result{}
	}
	r := Report{
		SchemaVersion: SchemaVersion,
		Tool:          Tool{Name: "rulecheck", Version: version.Current},
		Command:       "scan", Coverage: coverage, Results: results,
	}
	for i := range r.Results {
		result := &r.Results[i]
		slices.Sort(result.Aliases)
		result.Aliases = slices.Compact(result.Aliases)
		slices.SortFunc(result.Links, func(a, b Link) int {
			return cmp.Or(cmp.Compare(a.Path, b.Path), cmp.Compare(a.Target, b.Target))
		})
		result.Links = slices.Compact(result.Links)
		if result.Findings == nil {
			result.Findings = []Finding{}
		}
		for j := range result.Findings {
			finding := &result.Findings[j]
			if finding.Paths == nil {
				finding.Paths = []string{}
			}
			slices.Sort(finding.Paths)
			switch finding.Category {
			case Error:
				r.Counts.Error++
			case Warning:
				r.Counts.Warning++
			case Unknown:
				r.Counts.Unknown++
			case Skipped:
				r.Counts.Skipped++
			}
		}
		slices.SortFunc(result.Findings, compareFindings)
	}
	slices.SortFunc(r.Results, func(a, b Result) int {
		return cmp.Or(cmp.Compare(a.Kind, b.Kind), cmp.Compare(a.Target, b.Target))
	})
	switch {
	case r.Counts.Unknown > 0:
		r.Status = "incomplete"
	case r.Counts.Error > 0:
		r.Status = "failed"
	case r.Counts.Warning > 0:
		r.Status = "warning"
	default:
		r.Status = "passed"
	}
	return r
}

// SummarizeSkipped は対象外の詳細をコード別の件数に置き換えた結果を返す。
// 元の結果、分類別件数、状態、検査範囲は変更しない。
func (r Report) SummarizeSkipped() Report {
	r.Results = slices.Clone(r.Results)
	counts := make(map[string]int)
	for i := range r.Results {
		result := &r.Results[i]
		findings := make([]Finding, 0, len(result.Findings))
		for _, finding := range result.Findings {
			if finding.Category == Skipped {
				counts[finding.Code]++
			} else {
				findings = append(findings, finding)
			}
		}
		result.Findings = findings
	}
	for code, count := range counts {
		r.SkippedSummary = append(r.SkippedSummary, SkippedCount{Code: code, Count: count})
	}
	slices.SortFunc(r.SkippedSummary, func(a, b SkippedCount) int {
		return cmp.Compare(a.Code, b.Code)
	})
	return r
}

// SummarizeProblems は検査対象数を残し、問題のある対象だけを返す。
// 対象外の診断は種類別件数にまとめ、分類別件数・状態・終了コードを保つ。
func (r Report) SummarizeProblems() Report {
	total := len(r.Results)
	r = r.SummarizeSkipped()
	r.TargetCount = &total
	results := make([]Result, 0, len(r.Results))
	for _, result := range r.Results {
		if len(result.Findings) > 0 {
			results = append(results, result)
		}
	}
	r.Results = results
	return r
}

func compareFindings(a, b Finding) int {
	return cmp.Or(
		cmp.Compare(a.Source, b.Source), cmp.Compare(a.Line, b.Line),
		cmp.Compare(a.Code, b.Code), cmp.Compare(a.Category, b.Category),
		cmp.Compare(a.Message, b.Message),
		cmp.Compare(strings.Join(a.Paths, "\x00"), strings.Join(b.Paths, "\x00")),
	)
}

func InvalidArguments() Report {
	r := New(Coverage{Rules: NotRun, References: NotRun, Skills: NotRun, Commands: NotRun}, []Result{{
		Kind: "arguments",
		Findings: []Finding{{
			Code: "invalid_arguments", Category: Error,
			Message: "引数が正しくありません。rulecheck --help で使い方を確認してください。",
		}},
	}})
	r.Status = "invalid_arguments"
	return r
}

func (r Report) ExitCode() int {
	switch r.Status {
	case "invalid_arguments":
		return 64
	case "incomplete":
		return 2
	case "failed":
		return 1
	case "passed", "warning":
		return 0
	default:
		return 2
	}
}
