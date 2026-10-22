// Package cli は rulecheck の引数と標準入出力を扱う。
package cli

import (
	"fmt"
	"io"

	"rulecheck/internal/version"
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

現在は初期設定段階です。scan の引数処理・検査・JSON出力は未実装で、終了コード2を返します。
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
		// 未実装の検査を、検査完了や引数検証済みとして扱わない。
		fmt.Fprintln(stderr, "検査機能は未実装です。開発計画は docs/development-plan.md を参照してください。")
		return 2
	default:
		return invalidArguments(stderr)
	}
}

func invalidArguments(stderr io.Writer) int {
	fmt.Fprintln(stderr, "引数が正しくありません。rulecheck --help で使い方を確認してください。")
	return 64
}
