package cli

import (
	"strings"

	"rulecheck/internal/report"
)

type scanOptions struct {
	rules    []string
	skills   []string
	commands []string
	json     bool
}

// parseScan は値を実行せずに検証し、不正な値自体は診断へ転載しない。
func parseScan(args []string) (scanOptions, bool) {
	var options scanOptions
	valid := true
	for i := 0; i < len(args); i++ {
		key, value, assigned := strings.Cut(args[i], "=")
		switch key {
		case "--json":
			if assigned {
				valid = false
			} else {
				options.json = true
			}
		case "--rules", "--skills", "--command":
			if !assigned {
				if i+1 == len(args) || strings.HasPrefix(args[i+1], "-") {
					valid = false
					continue
				}
				i++
				value = args[i]
			}
			if strings.TrimSpace(value) == "" || strings.ContainsRune(value, 0) {
				valid = false
				continue
			}
			switch key {
			case "--rules":
				options.rules = append(options.rules, value)
			case "--skills":
				options.skills = append(options.skills, value)
			case "--command":
				if value == "." || value == ".." || strings.ContainsAny(value, `/\:`) {
					valid = false
					continue
				}
				options.commands = append(options.commands, value)
			}
		default:
			valid = false
		}
	}
	if len(options.rules)+len(options.skills)+len(options.commands) == 0 {
		valid = false
	}
	return options, valid
}

func pendingReport(options scanOptions) report.Report {
	coverage := report.Coverage{
		Rules: report.NotRequested, References: report.NotRequested,
		Skills: report.NotRequested, Commands: report.NotRequested,
	}
	var results []report.Result
	for _, group := range []struct {
		kind    string
		targets []string
		message string
	}{
		{"rules", options.rules, "指示ファイルと共有リンクの検査は未実装です。"},
		{"skills", options.skills, "スキルの列挙と構造の検査は未実装です。"},
		{"command", options.commands, "PATHからCLIを探す検査は未実装です。"},
	} {
		seen := make(map[string]bool)
		for _, target := range group.targets {
			if seen[target] {
				continue
			}
			seen[target] = true
			finding := report.Finding{
				Code: group.kind + "_not_implemented", Category: report.Unknown,
				Message: group.message,
			}
			if group.kind != "command" {
				finding.Paths = []string{target}
			}
			if group.kind == "rules" {
				finding.Source = target
			}
			results = append(results, report.Result{
				Kind: group.kind, Target: target, Findings: []report.Finding{finding},
			})
		}
	}
	if len(options.rules) != 0 {
		coverage.Rules = report.NotImplemented
		coverage.References = report.NotImplemented
	}
	if len(options.skills) != 0 {
		coverage.Skills = report.NotImplemented
		coverage.References = report.NotImplemented
	}
	if len(options.commands) != 0 {
		coverage.Commands = report.NotImplemented
	}
	return report.New(coverage, results)
}
