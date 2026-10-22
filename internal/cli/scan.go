package cli

import (
	"strings"
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
