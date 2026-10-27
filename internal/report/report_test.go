package report_test

import (
	"encoding/json"
	"testing"

	"github.com/shouhon/rulecheck/internal/report"
)

func TestClassificationAndExitPriority(t *testing.T) {
	for _, tt := range []struct {
		name       string
		categories []report.Category
		status     string
		code       int
		counts     report.Counts
	}{
		{"指摘なし", nil, "passed", 0, report.Counts{}},
		{"対象外のみ", []report.Category{report.Skipped}, "passed", 0, report.Counts{Skipped: 1}},
		{"注意と対象外", []report.Category{report.Warning, report.Skipped}, "warning", 0, report.Counts{Warning: 1, Skipped: 1}},
		{"エラー優先", []report.Category{report.Error, report.Warning}, "failed", 1, report.Counts{Error: 1, Warning: 1}},
		{"判定不能のみ", []report.Category{report.Unknown}, "incomplete", 2, report.Counts{Unknown: 1}},
		{"エラー併存でも判定不能優先", []report.Category{report.Error, report.Unknown, report.Warning, report.Skipped}, "incomplete", 2, report.Counts{Error: 1, Unknown: 1, Warning: 1, Skipped: 1}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var findings []report.Finding
			for _, category := range tt.categories {
				findings = append(findings, report.Finding{Category: category})
			}
			result := report.New(report.Coverage{}, []report.Result{{Kind: "rules", Target: "模擬.md", Findings: findings}})
			if result.Status != tt.status || result.ExitCode() != tt.code || result.Counts != tt.counts {
				t.Fatalf("分類と終了コードが一致しません: %+v、終了コード=%d", result, result.ExitCode())
			}
		})
	}
}

func TestStableFindingOrder(t *testing.T) {
	build := func(reverse bool) []byte {
		findings := []report.Finding{
			{Code: "reference_missing", Category: report.Error, Source: "模擬.md", Line: 9, Paths: []string{"b", "a"}},
			{Code: "reference_missing", Category: report.Error, Source: "模擬.md", Line: 3},
			{Code: "metadata_missing", Category: report.Error, Source: "模擬.md", Line: 3},
		}
		if reverse {
			findings[0], findings[2] = findings[2], findings[0]
			findings[2].Paths = []string{"a", "b"}
		}
		result := report.New(report.Coverage{}, []report.Result{{Kind: "rules", Target: "模擬.md", Findings: findings}})
		if result.Results[0].Findings[0].Code != "metadata_missing" || result.Results[0].Findings[2].Line != 9 {
			t.Fatalf("記述元・行番号・codeの順で並んでいません: %+v", result.Results)
		}
		data, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	if string(build(false)) != string(build(true)) {
		t.Fatal("診断と関係するパスの列挙順によりJSONが変わります")
	}
}

func TestSummaryPreservesOriginal(t *testing.T) {
	original := report.New(report.Coverage{}, []report.Result{{
		Kind: "rules", Target: "模擬.md", Findings: []report.Finding{
			{Code: "reference_external_skipped", Category: report.Skipped},
			{Code: "reference_missing", Category: report.Error},
		},
	}})
	before, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	summary := original.SummarizeSkipped()
	quiet := original.SummarizeProblems()
	after, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) || len(summary.Results[0].Findings) != 1 || len(quiet.Results[0].Findings) != 1 {
		t.Fatal("要約が元の詳細出力を変更しています")
	}
}
