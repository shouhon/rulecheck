// Package cli は rulecheck の引数と標準入出力を扱う。
package cli

import (
	"fmt"
	"io"

	"github.com/shouhon/rulecheck/internal/report"
	"github.com/shouhon/rulecheck/internal/scan"
	"github.com/shouhon/rulecheck/internal/version"
)

const help = `rulecheck は、明示された指示ファイル・スキル・CLIの存在関係を検査するツールです。

使い方:
  rulecheck --help
  rulecheck --version
  rulecheck scan [--rules <ファイル> ...] [--skills <親ディレクトリ> ...] [--command <名前> ...] [--json]

検査対象の指定（少なくとも1つ必要）:
  --rules    指示ファイル。複数指定できます。
  --skills   スキルの親ディレクトリ。直下のスキルだけを対象とします。
  --command  PATHで探すCLI名。CLI自体は実行しません。
  --json     結果をJSONで出力します。

検査範囲:
  明示した対象だけを読み取り、ファイルや設定の変更、外部URLへの接続は行いません。

ファイルと共有リンク、Markdownのローカル参照、直下のスキルの基本項目・名前、PATHのCLI候補検索に対応しています。
相対参照は記述元の実体ディレクトリを基準に解決し、参照先の本文は読みません。
CLIの候補検索は起動・認証・実行権限を保証しません。
未対応の参照や読取不能は判定不能として残し、終了コード2を返します。
`

// Run は処理結果の終了コードを返す。プロセスの終了は呼び出し元が行う。
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, help)
		return 64
	}

	switch args[0] {
	case "--help", "-h":
		if len(args) != 1 {
			return invalidArguments(stderr)
		}
		fmt.Fprint(stdout, help)
		return 0
	case "--version":
		if len(args) != 1 {
			return invalidArguments(stderr)
		}
		fmt.Fprintf(stdout, "rulecheck %s\n", version.Current)
		return 0
	case "scan":
		if len(args) == 2 && (args[1] == "--help" || args[1] == "-h") {
			fmt.Fprint(stdout, help)
			return 0
		}
		options, valid := parseScan(args[1:])
		if !valid {
			invalidArguments(stderr)
			if options.json {
				writeReport(report.InvalidArguments(), true, stdout, stderr)
			}
			return 64
		}
		result := scan.Run(scan.Options{Rules: options.rules, Skills: options.skills, Commands: options.commands})
		return writeReport(result, options.json, stdout, stderr)
	default:
		return invalidArguments(stderr)
	}
}

func invalidArguments(stderr io.Writer) int {
	fmt.Fprintln(stderr, "引数が正しくありません。rulecheck --help で使い方を確認してください。")
	return 64
}
