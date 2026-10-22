package main_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"rulecheck/internal/report"
	"rulecheck/internal/version"
)

const secret = "acceptance-secret-do-not-print"

// TestExecutableAcceptance は模擬データだけを使い、プロセス境界で受入条件を確認する。
func TestExecutableAcceptance(t *testing.T) {
	binary := buildAcceptanceBinary(t)

	t.Run("正常な共有リンクと実体基準の参照", func(t *testing.T) {
		f := newFixture(t, binary)
		writeFile(t, f.path("実体", "参照先 空白.md"), secret+"\x00\n[再帰しない](missing.md)\n")
		writeFile(t, f.path("実体", "AGENTS.md"), "[通常](<参照先 空白.md>)\n[参照形式][資料]\n![画像](参照先%20空白.md)\n\n[資料]: <参照先 空白.md>\n")
		alias := f.path("別名", "GEMINI.md")
		makeSymlink(t, filepath.Join("..", "実体", "AGENTS.md"), alias)
		watchFixture(t, f.root)
		lockFile(t, f.path("実体", "参照先 空白.md"))
		lockFile(t, f.path("tools", "mock.cmd"))
		args := []string{"--rules", alias, "--rules", f.path("実体", "AGENTS.md"), "--command=mock"}
		r, first := f.check(t, args, "passed", 0, report.Counts{})
		if r.Coverage != (report.Coverage{Rules: report.Completed, References: report.Completed, Skills: report.NotRequested, Commands: report.Completed}) {
			t.Fatal("検査範囲が実施した内容と一致しません")
		}
		rules := resultOf(t, r, "rules")
		if len(rules) != 1 || rules[0].ResolvedPath != f.path("実体", "AGENTS.md") || len(rules[0].Aliases) != 2 || len(rules[0].Links) != 1 {
			t.Fatal("共有リンクの実体・別名・参照関係を保持していません")
		}
		_, second := f.check(t, []string{"--command=mock", "--rules", f.path("実体", "AGENTS.md"), "--rules", alias, "--rules", alias}, "passed", 0, report.Counts{})
		if first != second {
			t.Fatal("引数の順序や同じ対象の再指定でJSONが変わりました")
		}
	})

	t.Run("リンク切れと循環と読取不能", func(t *testing.T) {
		f := newFixture(t, binary)
		broken, cycle, unreadable := f.path("切れたリンク.md"), f.path("循環.md"), f.path("読取不能.md")
		makeSymlink(t, "missing.md", broken)
		makeSymlink(t, filepath.Base(cycle), cycle)
		writeFile(t, unreadable, secret)
		alias := f.path("読取不能の別名.md")
		makeSymlink(t, filepath.Base(unreadable), alias)
		watchFixture(t, f.root)
		lockFile(t, unreadable)
		for _, tc := range []struct {
			path, status, finding string
			code                  int
			counts                report.Counts
		}{
			{broken, "failed", "link_broken", 1, report.Counts{Error: 1}},
			{cycle, "failed", "link_cycle", 1, report.Counts{Error: 1}},
			{alias, "incomplete", "path_unreadable", 2, report.Counts{Unknown: 1}},
		} {
			r, _ := f.check(t, []string{"--rules", tc.path}, tc.status, tc.code, tc.counts)
			requireFinding(t, r, tc.finding)
		}
		f.check(t, []string{"--rules", broken, "--rules", alias}, "incomplete", 2, report.Counts{Error: 1, Unknown: 1})
	})

	t.Run("未対応の再解析ポイントを判定不能にする", func(t *testing.T) {
		f := newFixture(t, binary)
		for _, path := range []string{f.path("特殊.md"), f.path("tools", "unsupported.cmd"), f.path("skills", "file", "SKILL.md")} {
			writeFile(t, path, secret)
			makeUnsupportedReparsePoint(t, path)
		}
		for _, path := range []string{f.path("特殊ディレクトリ"), f.path("skills", "directory")} {
			if err := os.MkdirAll(path, 0700); err != nil {
				t.Fatal(err)
			}
			makeUnsupportedReparsePoint(t, path)
		}
		writeFile(t, f.path("AGENTS.md"), "[ファイル](特殊.md)\n[ディレクトリ](特殊ディレクトリ)\n[その中](特殊ディレクトリ/AGENTS.md)\n")
		makeSymlink(t, "特殊.md", f.path("別名.md"))
		watchFixture(t, f.root)
		for _, tc := range []struct {
			name, kind, finding string
			args                []string
			unknown             int
			coverage            report.Coverage
		}{
			{"指定ファイル", "rules", "path_unreadable", []string{"--rules=特殊.md"}, 1,
				report.Coverage{Rules: report.Partial, References: report.Partial, Skills: report.NotRequested, Commands: report.NotRequested}},
			{"共有リンクの参照先", "rules", "path_unreadable", []string{"--rules=別名.md"}, 1,
				report.Coverage{Rules: report.Partial, References: report.Partial, Skills: report.NotRequested, Commands: report.NotRequested}},
			{"途中のディレクトリ", "rules", "path_unreadable", []string{"--rules=特殊ディレクトリ/AGENTS.md"}, 1,
				report.Coverage{Rules: report.Partial, References: report.Partial, Skills: report.NotRequested, Commands: report.NotRequested}},
			{"スキルの親", "skills", "path_unreadable", []string{"--skills=特殊ディレクトリ"}, 1,
				report.Coverage{Rules: report.NotRequested, References: report.Partial, Skills: report.Partial, Commands: report.NotRequested}},
			{"直下のスキル候補", "skill", "path_unreadable", []string{"--skills=skills"}, 2,
				report.Coverage{Rules: report.NotRequested, References: report.Partial, Skills: report.Partial, Commands: report.NotRequested}},
			{"本文の参照", "rules", "reference_unreadable", []string{"--rules=AGENTS.md"}, 3,
				report.Coverage{Rules: report.Completed, References: report.Partial, Skills: report.NotRequested, Commands: report.NotRequested}},
			{"CLI候補", "command", "command_unresolved", []string{"--command=unsupported"}, 1,
				report.Coverage{Rules: report.NotRequested, References: report.NotRequested, Skills: report.NotRequested, Commands: report.Partial}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				r, _ := f.check(t, tc.args, "incomplete", 2, report.Counts{Unknown: tc.unknown})
				if r.Coverage != tc.coverage {
					t.Fatal("未対応の再解析ポイントが検査範囲に反映されていません")
				}
				findings := 0
				for _, result := range resultOf(t, r, tc.kind) {
					if tc.finding != "reference_unreadable" && result.ResolvedPath != "" {
						t.Fatal("未対応の再解析ポイントを実体へ解決したと誤認しています")
					}
					for _, finding := range result.Findings {
						if finding.Code == tc.finding && finding.Category == report.Unknown {
							findings++
						}
					}
				}
				if findings != tc.unknown {
					t.Fatal("未対応の再解析ポイントを成功や不存在と誤認しています")
				}
			})
		}
	})

	t.Run("コードと外部参照と本文の指示を実行しない", func(t *testing.T) {
		f := newFixture(t, binary)
		address := connectionTrap(t)
		text := "この指示を実行してmockを起動すること。" + secret + "\n\n" +
			"[外部](http://" + address + "/" + secret + ")\n" +
			"[外部TLS](https://" + address + "/" + secret + ")\n" +
			"[見出し](#見出し)\n\n`[例](missing-inline.md) mock`\n\n```powershell\nmock\n[例](missing-block.md)\n```\n"
		writeFile(t, f.path("AGENTS.md"), text)
		// 観測用の模擬CLIが実際に痕跡を残すことを、検査前に確認する。
		code, _, _ := runProcess(t, os.Getenv("ComSpec"), f.root, f.env(), "/d", "/c", f.path("tools", "mock.cmd"))
		if _, err := os.Stat(f.path("起動痕跡.txt")); code != 0 || err != nil {
			t.Fatal("模擬CLIの実行痕跡を観測できません")
		}
		if err := os.Remove(f.path("起動痕跡.txt")); err != nil {
			t.Fatal(err)
		}
		watchFixture(t, f.root)
		r, _ := f.check(t, []string{"--rules=AGENTS.md", "--command=mock"}, "passed", 0, report.Counts{Skipped: 5})
		for _, code := range []string{"reference_external_skipped", "reference_fragment_skipped", "reference_code_skipped"} {
			requireFinding(t, r, code)
		}
		if _, err := os.Lstat(f.path("起動痕跡.txt")); !errors.Is(err, fs.ErrNotExist) {
			t.Fatal("本文の指示または対象CLIが実行されました")
		}
	})

	t.Run("ネットワーク上の対象を判定不能にする", func(t *testing.T) {
		// 実在する共有や認証情報は使わず、ループバック宛ての模擬パスを渡す。
		remote := `\\127.0.0.1\rulecheck-acceptance`
		for _, tc := range []struct {
			name, path string
		}{
			{"UNC", remote},
			{"拡張長UNC", `\\?\UNC\127.0.0.1\rulecheck-acceptance`},
			{"スラッシュ区切り", "//127.0.0.1/rulecheck-acceptance"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				f := newFixture(t, binary)
				watchFixture(t, f.root)
				for _, option := range []string{"--rules", "--skills"} {
					r, _ := f.check(t, []string{option, tc.path}, "incomplete", 2, report.Counts{Unknown: 1})
					finding := requireFinding(t, r, "path_not_local")
					if finding.Category != report.Unknown || len(r.Results) != 1 || r.Results[0].ResolvedPath != "" {
						t.Fatal("ネットワーク上の指定をローカルの実体として扱っています")
					}
				}
			})
		}
		t.Run("本文に書かれたネットワーク参照", func(t *testing.T) {
			f := newFixture(t, binary)
			writeFile(t, f.path("AGENTS.md"), "[通常](//127.0.0.1/rulecheck-acceptance/資料.md)\n"+
				"[参照形式][共有]\n![画像](%5C%5C127.0.0.1%5Crulecheck-acceptance%5C資料.png)\n\n"+
				"[共有]: <"+remote+"\\資料.md>\n")
			watchFixture(t, f.root)
			r, _ := f.check(t, []string{"--rules=AGENTS.md"}, "incomplete", 2, report.Counts{Unknown: 3})
			if r.Coverage.Rules != report.Completed || r.Coverage.References != report.Partial {
				t.Fatal("ネットワーク参照の判定不能が検査範囲に反映されていません")
			}
			for i, finding := range resultOf(t, r, "rules")[0].Findings {
				if finding.Code != "reference_unsupported" || finding.Category != report.Unknown || finding.Line != i+1 || len(finding.Paths) != 0 {
					t.Fatal("ネットワーク参照の種類・記述位置が一致しません")
				}
			}
		})
		checkNetworkLinks(t, binary, remote)
	})

	t.Run("メタデータと直下だけの列挙", func(t *testing.T) {
		f := newFixture(t, binary)
		for name, metadata := range map[string]string{
			"syntax":      "name: [\ndescription: " + secret,
			"missing":     "description: " + secret,
			"missingdesc": "name: missingdesc",
			"typed":       "name: 123\ndescription: false",
			"valid":       "name: valid\ndescription: 説明\nextra: " + secret,
		} {
			writeFile(t, f.path("skills", name, "SKILL.md"), "---\n"+metadata+"\n---\n")
		}
		writeFile(t, f.path("skills", "補助", "入れ子", "SKILL.md"), secret)
		watchFixture(t, f.root)
		r, _ := f.check(t, []string{"--skills=skills"}, "failed", 1, report.Counts{Error: 5})
		if len(resultOf(t, r, "skill")) != 5 || r.Coverage.Skills != report.Completed || r.Coverage.References != report.Completed {
			t.Fatal("直下のスキルだけを検査していません")
		}
		for _, code := range []string{"metadata_invalid", "metadata_name_missing", "metadata_description_missing", "metadata_name_invalid", "metadata_description_invalid"} {
			requireFinding(t, r, code)
		}
	})

	t.Run("同名の異なる実体と同一実体の別名", func(t *testing.T) {
		f := newFixture(t, binary)
		for _, parent := range []string{"one", "two"} {
			writeFile(t, f.path(parent, "same", "SKILL.md"), "---\nname: same\ndescription: "+secret+"\n---\n")
		}
		makeSymlink(t, "same", f.path("one", "alias"))
		watchFixture(t, f.root)
		r, _ := f.check(t, []string{"--skills=one"}, "passed", 0, report.Counts{})
		skills := resultOf(t, r, "skill")
		if len(skills) != 1 || len(skills[0].Aliases) != 2 {
			t.Fatal("同じ実体のスキルを集約できません")
		}
		r, _ = f.check(t, []string{"--skills=one", "--skills=two"}, "warning", 0, report.Counts{Warning: 2})
		for _, skill := range resultOf(t, r, "skill") {
			if len(skill.Findings) != 1 || skill.Findings[0].Code != "skill_name_duplicate" || len(skill.Findings[0].Paths) != 2 {
				t.Fatal("異なる実体の同名スキルを注意として区別できません")
			}
		}
	})

	t.Run("参照エラーとCLI不足と未対応表記", func(t *testing.T) {
		f := newFixture(t, binary)
		writeFile(t, f.path("AGENTS.md"), "本文\n[不在](missing.md)\n")
		writeFile(t, f.path("未対応.md"), "[[独自参照]]\n")
		watchFixture(t, f.root)
		r, output := f.check(t, []string{"--rules=AGENTS.md", "--command=missing"}, "failed", 1, report.Counts{Error: 2})
		finding := requireFinding(t, r, "reference_missing")
		if finding.Line != 2 || finding.Source != f.path("AGENTS.md") || len(finding.Paths) != 1 || finding.Paths[0] != f.path("missing.md") {
			t.Fatal("参照エラーの記述位置と関係するパスが一致しません")
		}
		if !strings.Contains(output.text, "行 2") || !strings.Contains(output.text, "missing.md") {
			t.Fatal("通常表示に記述位置と関係するパスがありません")
		}
		requireFinding(t, r, "command_missing")
		r, _ = f.check(t, []string{"--rules=未対応.md", "--command=missing"}, "incomplete", 2, report.Counts{Error: 1, Unknown: 1})
		if r.Coverage.Rules != report.Completed || r.Coverage.References != report.Partial || r.Coverage.Commands != report.Completed {
			t.Fatal("参照の判定不能が他の検査範囲を変えています")
		}
	})

	t.Run("不正引数のJSONと出力先", func(t *testing.T) {
		f := newFixture(t, binary)
		watchFixture(t, f.root)
		for _, args := range [][]string{{}, {"--unknown=" + secret}} {
			code, stdout, stderr := f.run(t, append(append([]string{"scan"}, args...), "--json")...)
			r := decodeReport(t, stdout, "invalid_arguments", code, 64, report.Counts{Error: 1})
			if !strings.Contains(stderr, "引数が正しくありません") || r.Coverage != (report.Coverage{Rules: report.NotRun, References: report.NotRun, Skills: report.NotRun, Commands: report.NotRun}) {
				t.Fatal("引数誤りの出力先または検査範囲が一致しません")
			}
			code, stdout, stderr = f.run(t, append([]string{"scan"}, args...)...)
			if code != 64 || stdout != "" || !strings.Contains(stderr, "引数が正しくありません") {
				t.Fatal("通常表示の引数誤りが標準エラーだけに出ていません")
			}
		}
	})
}

func buildAcceptanceBinary(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "rulecheck.exe")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "build", "-trimpath", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("受入確認用のビルドに失敗しました: %v\n%s", err, output)
	}
	body, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("検証対象: CLI %s、実行ファイルSHA-256 %x", version.Current, sha256.Sum256(body))
	if code, stdout, stderr := runProcess(t, binary, t.TempDir(), nil, "--version"); code != 0 || stdout != "rulecheck "+version.Current+"\n" || stderr != "" {
		t.Fatal("実行ファイルの版番号が正本と一致しません")
	}
	return binary
}

func checkNetworkLinks(t *testing.T, binary, remote string) {
	t.Helper()
	for _, tc := range []struct {
		name, link, target, finding string
		args                        []string
		directory                   bool
	}{
		{"指定ファイルのリンク", "別名.md", remote + `\AGENTS.md`, "path_not_local", []string{"--rules=別名.md"}, false},
		{"スキルの親のリンク", "skills", remote, "path_not_local", []string{"--skills=skills"}, true},
		{"直下のスキルのリンク", "skills/remote", remote, "path_not_local", []string{"--skills=skills"}, true},
		{"スキルファイルのリンク", "skills/remote/SKILL.md", remote + `\SKILL.md`, "path_not_local", []string{"--skills=skills"}, false},
		{"本文から参照するリンク", "別名.md", remote + `\資料.md`, "reference_not_local", []string{"--rules=AGENTS.md"}, false},
		{"PATHのリンク", "tools", remote, "command_unresolved", []string{"--command=mock"}, true},
		{"CLI候補のリンク", "tools/remote.cmd", remote + `\mock.cmd`, "command_unresolved", []string{"--command=remote"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := fixture{t.TempDir(), binary}
			writeFile(t, f.path("AGENTS.md"), "[資料](別名.md)\n")
			writeFile(t, f.path("profile", "設定.txt"), secret)
			makeRemoteSymlink(t, tc.target, f.path(tc.link), tc.directory)
			watchFixture(t, f.root)
			r, _ := f.check(t, tc.args, "incomplete", 2, report.Counts{Unknown: 1})
			requireFinding(t, r, tc.finding)
		})
	}
}

type fixture struct{ root, binary string }

func newFixture(t *testing.T, binary string) fixture {
	t.Helper()
	f := fixture{t.TempDir(), binary}
	writeFile(t, f.path("tools", "mock.cmd"), "@echo off\r\necho started>\"%RULECHECK_ACCEPTANCE_MARKER%\"\r\n")
	writeFile(t, f.path("profile", "設定.txt"), secret)
	return f
}

func (f fixture) path(parts ...string) string {
	return filepath.Join(append([]string{f.root}, parts...)...)
}

func (f fixture) env() map[string]string {
	return map[string]string{
		"PATH": f.path("tools"), "PATHEXT": ".CMD", "RULECHECK_ACCEPTANCE_MARKER": f.path("起動痕跡.txt"),
		"HOME": f.path("profile"), "USERPROFILE": f.path("profile"), "APPDATA": f.path("profile"),
		"LOCALAPPDATA": f.path("profile"), "XDG_CONFIG_HOME": f.path("profile"),
		"TEMP": f.path("profile"), "TMP": f.path("profile"),
	}
}

func (f fixture) run(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	return runProcess(t, f.binary, f.root, f.env(), args...)
}

func runProcess(t *testing.T, binary, dir string, overrides map[string]string, args ...string) (int, string, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, args...)
	command.Dir = dir
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		replaced := false
		for override := range overrides {
			if strings.EqualFold(key, override) {
				replaced = true
				break
			}
		}
		if !replaced {
			command.Env = append(command.Env, entry)
		}
	}
	for key, value := range overrides {
		command.Env = append(command.Env, key+"="+value)
	}
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	started := time.Now().UTC()
	err := command.Start()
	if err == nil {
		err = command.Wait()
	}
	if os.Getenv("RULECHECK_TEST_TRACE_PROCESSES") == "1" && command.ProcessState != nil {
		t.Logf("観測区間: 実行ファイル=%s PID=%d 開始=%s 終了=%s 終了コード=%d", filepath.Base(binary), command.Process.Pid,
			started.Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano), command.ProcessState.ExitCode())
	}
	var exitError *exec.ExitError
	if ctx.Err() != nil || err != nil && !errors.As(err, &exitError) {
		t.Fatalf("実行ファイルを起動または完了できません: %v", err)
	}
	if strings.Contains(stdout.String()+stderr.String(), secret) {
		t.Fatal("模擬の機密値が出力へ漏れました")
	}
	return command.ProcessState.ExitCode(), stdout.String(), stderr.String()
}

type scanOutput struct{ json, text string }

func (f fixture) check(t *testing.T, args []string, status string, wantCode int, counts report.Counts) (report.Report, scanOutput) {
	t.Helper()
	code, stdout, stderr := f.run(t, append(append([]string{"scan"}, args...), "--json")...)
	if stderr != "" {
		t.Fatal("JSON出力に不要な標準エラーがあります")
	}
	r := decodeReport(t, stdout, status, code, wantCode, counts)
	jsonOutput := stdout
	code, stdout, stderr = f.run(t, append([]string{"scan"}, args...)...)
	label := map[string]string{"passed": "検査完了", "warning": "注意あり", "failed": "エラーあり", "incomplete": "検査未完了"}[status]
	countText := fmt.Sprintf("件数: エラー %d / 注意 %d / 判定不能 %d / 対象外 %d\n", counts.Error, counts.Warning, counts.Unknown, counts.Skipped)
	if code != wantCode || stderr != "" || !strings.HasPrefix(stdout, label+"\n") || !strings.HasSuffix(stdout, countText) {
		t.Fatal("通常表示の状態・件数・終了コードがJSONと一致しません")
	}
	return r, scanOutput{json: jsonOutput, text: stdout}
}

func decodeReport(t *testing.T, output, status string, code, wantCode int, counts report.Counts) report.Report {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(output))
	decoder.DisallowUnknownFields()
	var r report.Report
	if err := decoder.Decode(&r); err != nil {
		t.Fatalf("JSONを読み取れません: %v", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatal("標準出力がJSONオブジェクト1件ではありません")
	}
	if r.Tool.Name != "rulecheck" || r.Tool.Version != version.Current || r.SchemaVersion != 1 || r.Command != "scan" || r.Results == nil {
		t.Fatal("JSONの識別情報または必須の配列が一致しません")
	}
	if code != wantCode || r.Status != status || r.Counts != counts {
		t.Fatalf("JSONの状態・件数・終了コードが一致しません: 状態=%s 件数=%+v 終了コード=%d", r.Status, r.Counts, code)
	}
	var actual report.Counts
	for _, target := range r.Results {
		if target.Findings == nil {
			t.Fatal("診断の配列がnullです")
		}
		for _, finding := range target.Findings {
			if finding.Code == "" || finding.Message == "" || finding.Paths == nil {
				t.Fatal("診断の必須項目がありません")
			}
			switch finding.Category {
			case report.Error:
				actual.Error++
			case report.Warning:
				actual.Warning++
			case report.Unknown:
				actual.Unknown++
			case report.Skipped:
				actual.Skipped++
			default:
				t.Fatal("未定義の分類があります")
			}
		}
	}
	if actual != counts {
		t.Fatal("個々の診断を数えた件数とJSONの集計が一致しません")
	}
	return r
}

func resultOf(t *testing.T, r report.Report, kind string) []report.Result {
	t.Helper()
	var results []report.Result
	for _, result := range r.Results {
		if result.Kind == kind {
			results = append(results, result)
		}
	}
	return results
}

func requireFinding(t *testing.T, r report.Report, code string) report.Finding {
	t.Helper()
	for _, result := range r.Results {
		for _, finding := range result.Findings {
			if finding.Code == code {
				return finding
			}
		}
	}
	t.Fatalf("必要な診断 %s がありません", code)
	return report.Finding{}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

func makeSymlink(t *testing.T, target, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("共有リンクを作成できないため、この受入条件は未確認: %v", err)
	}
}

func makeRemoteSymlink(t *testing.T, target, path string, directory bool) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	destination, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		t.Fatal(err)
	}
	// 種類を明示し、作成側でもネットワーク先の属性を確認しない。
	flags := uint32(2) // 開発者モードでのリンク作成を許す。
	if directory {
		flags |= 1
	}
	if err := syscall.CreateSymbolicLink(name, destination, flags); err != nil {
		t.Skipf("ネットワーク向けの共有リンクを作成できないため未確認: %v", err)
	}
}

func lockFile(t *testing.T, path string) {
	t.Helper()
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := syscall.CreateFile(name, syscall.GENERIC_READ, 0, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := syscall.CloseHandle(handle); err != nil {
			t.Fatal(err)
		}
	})
}

// makeUnsupportedReparsePoint はドライバーの導入なしに模擬タグとGUIDを設定する。
func makeUnsupportedReparsePoint(t *testing.T, path string) {
	t.Helper()
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := syscall.CreateFile(name, syscall.GENERIC_WRITE, 0, nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_OPEN_REPARSE_POINT|syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		t.Fatal(err)
	}
	data := make([]byte, 24+len(secret))
	binary.LittleEndian.PutUint32(data[:4], 0x00000042)
	binary.LittleEndian.PutUint16(data[4:6], uint16(len(secret)))
	copy(data[8:24], "rulecheck-test-01")
	copy(data[24:], secret)
	var returned uint32
	err = syscall.DeviceIoControl(handle, 0x000900a4, &data[0], uint32(len(data)), nil, 0, &returned, nil) // FSCTL_SET_REPARSE_POINT
	closeErr := syscall.CloseHandle(handle)
	if err != nil {
		if errors.Is(err, syscall.ERROR_ACCESS_DENIED) || errors.Is(err, syscall.ERROR_PRIVILEGE_NOT_HELD) || errors.Is(err, syscall.Errno(50)) || errors.Is(err, syscall.Errno(1)) {
			t.Skipf("再解析ポイントを作成できないため、この受入条件は未確認: %v", err)
		}
		t.Fatal(err)
	}
	t.Cleanup(func() {
		handle, err := syscall.CreateFile(name, syscall.GENERIC_WRITE, 0, nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_OPEN_REPARSE_POINT|syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
		if err != nil {
			t.Error(err)
			return
		}
		defer syscall.CloseHandle(handle)
		binary.LittleEndian.PutUint16(data[4:6], 0)
		if err := syscall.DeviceIoControl(handle, 0x000900ac, &data[0], 24, nil, 0, &returned, nil); err != nil { // FSCTL_DELETE_REPARSE_POINT
			t.Error(err)
		}
	})
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	actual, err := reparseData(path)
	if err != nil || !bytes.Equal(actual, data) {
		t.Fatal("模擬の再解析データを読み戻せません")
	}
}

func reparseData(path string) ([]byte, error) {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := syscall.CreateFile(name, 0, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE, nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_OPEN_REPARSE_POINT|syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return nil, err
	}
	defer syscall.CloseHandle(handle)
	data := make([]byte, syscall.MAXIMUM_REPARSE_DATA_BUFFER_SIZE)
	var returned uint32
	if err := syscall.DeviceIoControl(handle, syscall.FSCTL_GET_REPARSE_POINT, nil, 0, &data[0], uint32(len(data)), &returned, nil); err != nil {
		return nil, err
	}
	return data[:returned], nil
}

type fileState struct {
	Mode       fs.FileMode
	Modified   int64
	Attributes uint32
	Hash       [sha256.Size]byte
	Reparse    [sha256.Size]byte
}

func snapshot(t *testing.T, root string) map[string]fileState {
	t.Helper()
	states := make(map[string]fileState)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		state := fileState{Mode: info.Mode(), Modified: info.ModTime().UnixNano(), Attributes: info.Sys().(*syscall.Win32FileAttributeData).FileAttributes}
		if state.Attributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			data, err := reparseData(path)
			if err != nil {
				return err
			}
			state.Reparse = sha256.Sum256(data)
		} else if info.Mode().IsRegular() {
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			state.Hash = sha256.Sum256(body)
		}
		name, err := filepath.Rel(root, path)
		states[name] = state
		if err == nil && entry.IsDir() && state.Attributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return fs.SkipDir
		}
		return err
	})
	if err != nil {
		t.Fatalf("模擬データの状態を確認できません: %v", err)
	}
	return states
}

func watchFixture(t *testing.T, root string) {
	t.Helper()
	// 作成時刻の遅延反映を検査の変更と誤認しないよう、ディレクトリ時刻を固定する。
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || !entry.IsDir() {
			return err
		}
		if entry.Type()&fs.ModeIrregular != 0 {
			return fs.SkipDir
		}
		fixed := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		return os.Chtimes(path, fixed, fixed)
	})
	if err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, root)
	t.Cleanup(func() {
		if !reflect.DeepEqual(before, snapshot(t, root)) {
			t.Error("検査前後で模擬ファイル・リンク・設定・ディレクトリの状態が変わりました")
		}
	})
}

// connectionTrap は指定したHTTP・HTTPS参照へのTCP接続だけを観測する。
// DNS・SMB・別の接続先など、プロセスの全通信を監視するものではない。
func connectionTrap(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var count atomic.Int64
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			count.Add(1)
			_ = conn.SetWriteDeadline(time.Now().Add(time.Second))
			_, _ = conn.Write([]byte{1})
			_ = conn.Close()
		}
	}()
	probe := func() error {
		conn, err := net.DialTimeout("tcp4", listener.Addr().String(), time.Second)
		if err != nil {
			return err
		}
		defer conn.Close()
		_ = conn.SetReadDeadline(time.Now().Add(time.Second))
		_, err = io.ReadFull(conn, make([]byte, 1))
		return err
	}
	t.Cleanup(func() {
		if err := probe(); err != nil {
			t.Errorf("検査後の接続検知器を確認できません: %v", err)
		}
		_ = listener.Close()
		<-done
		if count.Load() != 2 {
			t.Errorf("前後の動作確認以外に外部参照への接続がありました: %d件", count.Load()-2)
		}
	})
	if err := probe(); err != nil {
		t.Fatalf("検査前の接続検知器を確認できません: %v", err)
	}
	return listener.Addr().String()
}
