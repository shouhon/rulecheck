package cli

import (
	"strings"
)

type scanOptions struct {
	rules     []string
	skills    []string
	skillDirs []string
	commands  []string
	json      bool
	summary   bool
	quiet     bool
}

// parseScan は値を実行せずに検証し、不正な値自体は診断へ転載しない。
func parseScan(args []string) (scanOptions, bool) {
	var options scanOptions
	valid := true
	for i := 0; i < len(args); i++ {
		key, value, assigned := strings.Cut(args[i], "=")
		switch key {
		case "--json", "--summary", "--quiet":
			if assigned {
				valid = false
			} else if key == "--json" {
				options.json = true
			} else if key == "--summary" {
				options.summary = true
			} else {
				options.quiet = true
			}
		case "--rules", "--skills", "--skill", "--command":
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
			case "--skill":
				options.skillDirs = append(options.skillDirs, value)
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
	if len(options.rules)+len(options.skills)+len(options.skillDirs)+len(options.commands) == 0 {
		valid = false
	}
	return options, valid
}
