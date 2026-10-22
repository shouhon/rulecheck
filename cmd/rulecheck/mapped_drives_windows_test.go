package main_test

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"unsafe"

	"rulecheck/internal/report"
)

// TestExecutableMappedDrives は明示された割当ドライブを使う。共有内には何も作らない。
func TestExecutableMappedDrives(t *testing.T) {
	value := os.Getenv("RULECHECK_TEST_NETWORK_DRIVES")
	if value == "" {
		t.Skip("割当ドライブの実機確認は未指定: RULECHECK_TEST_NETWORK_DRIVESにY:;Z:のように指定してください")
	}
	var roots []string
	seen := make(map[string]bool)
	getDriveType := syscall.NewLazyDLL("kernel32.dll").NewProc("GetDriveTypeW")
	for _, entry := range strings.Split(value, ";") {
		entry = strings.ToUpper(strings.TrimSpace(entry))
		if len(entry) != 2 || entry[0] < 'A' || entry[0] > 'Z' || entry[1] != ':' {
			t.Fatal("割当ドライブはドライブ名だけをセミコロンで区切って指定してください")
		}
		root := entry + `\`
		name, err := syscall.UTF16PtrFromString(root)
		if err != nil {
			t.Fatal(err)
		}
		// ルートの種別だけを問い合わせ、共有内の存在確認や列挙は行わない。
		kind, _, _ := getDriveType.Call(uintptr(unsafe.Pointer(name)))
		if kind != 4 {
			t.Fatalf("%sはこの実行環境でネットワークドライブとして見えていません（種別%d）", entry, kind)
		}
		if !seen[root] {
			roots = append(roots, root)
			seen[root] = true
		}
	}

	binary := buildAcceptanceBinary(t)
	for _, root := range roots {
		t.Run(root[:2], func(t *testing.T) {
			// 名前はローカルで生成するだけで、共有内に作成も存在確認もしない。
			remote := root + "rulecheck-acceptance-" + filepath.Base(t.TempDir())
			for _, tc := range []struct{ name, path string }{
				{"ルート", root},
				{"子パス", remote},
				{"スラッシュ区切り", strings.ReplaceAll(remote, `\`, "/")},
				{"拡張長パス", `\\?\` + remote},
			} {
				t.Run(tc.name, func(t *testing.T) {
					f := newFixture(t, binary)
					watchFixture(t, f.root)
					for _, option := range []string{"--rules", "--skills"} {
						r, _ := f.check(t, []string{option, tc.path}, "incomplete", 2, report.Counts{Unknown: 1})
						finding := requireFinding(t, r, "path_not_local")
						if finding.Category != report.Unknown || len(r.Results) != 1 || r.Results[0].ResolvedPath != "" {
							t.Fatal("割当ドライブの指定をローカルの実体として扱っています")
						}
					}
				})
			}
			t.Run("本文の参照", func(t *testing.T) {
				f := newFixture(t, binary)
				path := strings.ReplaceAll(remote, `\`, "/")
				writeFile(t, f.path("AGENTS.md"), "[通常]("+path+"/資料.md)\n[参照形式][共有]\n![画像]("+path+"/資料.png)\n\n[共有]: <"+path+"/資料.md>\n")
				watchFixture(t, f.root)
				r, _ := f.check(t, []string{"--rules=AGENTS.md"}, "incomplete", 2, report.Counts{Unknown: 3})
				if r.Coverage.Rules != report.Completed || r.Coverage.References != report.Partial {
					t.Fatal("割当ドライブの参照が検査範囲に反映されていません")
				}
				for i, finding := range resultOf(t, r, "rules")[0].Findings {
					if finding.Code != "reference_not_local" || finding.Category != report.Unknown || finding.Line != i+1 || len(finding.Paths) != 0 {
						t.Fatal("割当ドライブの参照の種類・記述位置が一致しません")
					}
				}
			})
			t.Run("PATHの指定", func(t *testing.T) {
				f := newFixture(t, binary)
				watchFixture(t, f.root)
				env := f.env()
				env["PATH"] = remote
				code, stdout, stderr := runProcess(t, binary, f.root, env, "scan", "--command=mock", "--json")
				if stderr != "" {
					t.Fatal("JSON出力に不要な標準エラーがあります")
				}
				r := decodeReport(t, stdout, "incomplete", code, 2, report.Counts{Unknown: 1})
				requireFinding(t, r, "command_unresolved")
				if r.Coverage.Commands != report.Partial || len(r.Results) != 1 || r.Results[0].ResolvedPath != "" {
					t.Fatal("割当ドライブのPATHを確認済みとして扱っています")
				}
				code, stdout, stderr = runProcess(t, binary, f.root, env, "scan", "--command=mock")
				if code != 2 || stderr != "" || !strings.HasPrefix(stdout, "検査未完了\n") || !strings.HasSuffix(stdout, "件数: エラー 0 / 注意 0 / 判定不能 1 / 対象外 0\n") {
					t.Fatal("PATHの通常表示とJSONの判定が一致しません")
				}
			})
			checkNetworkLinks(t, binary, remote)
		})
	}
}
