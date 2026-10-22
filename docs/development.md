# 開発と検証

Windows 11・PowerShell 7を使います。Goの最低版は[go.mod](../go.mod)が正本です。依存関係の選定と固定版は[依存関係](dependencies.md)を参照してください。初回ビルドには依存の取得が必要です。

## 必須検証

次の1コマンドを検証の入口とします。どの作業ディレクトリから呼び出しても、スクリプト自身がプロジェクトのルートへ移動します。

```powershell
pwsh -NoProfile -File ./scripts/check.ps1
```

[scripts/check.ps1](../scripts/check.ps1)は次を順番に実行し、失敗時に停止します。

| 確認 | コマンド |
| --- | --- |
| 書式 | `gofmt -l cmd internal`。出力があれば失敗 |
| モジュールの整合性 | `go mod tidy -diff`。定義ファイルは変更しない |
| 静的検査 | `go vet ./...` |
| テスト | `go test ./...` |
| ビルド | `go build -trimpath -o bin/rulecheck.exe ./cmd/rulecheck` |
| 起動 | `./bin/rulecheck.exe --version` |

書式の修正は`go fmt ./...`で行います。テストはCLIの契約に加え、一時ディレクトリ内でファイルの種類・読取可否、共有リンクと別名、循環・リンク切れ、直下だけのスキル列挙、Markdownの構文と相対参照の基準、YAMLの型・欠落・同名の区別を確認します。CLI候補は模擬の`PATH`と`PATHEXT`を使い、検索順序、候補の不足と判定不能、本文の非読取と非実行を確認します。Windowsでは読取を拒むファイルハンドルを使い、参照先の本文を開かないこと、特殊なパス、ジャンクションも確認します。リンクを作成できない場合は該当テストを未確認としてスキップするため、`go test -v ./internal/scan`で理由を確認してください。製品全体の受入確認の現在地は[開発計画](development-plan.md)に残します。

Windowsでは、`go test ./...`から実行ファイルを別プロセスとして起動する受入テストも実行します。模擬データを使い、対象の前後比較、参照先本文の非読取、CLIの非実行、模擬機密値の非出力、結果と終了コードの一致を確認します。受入テストだけの再実行方法と、通信の確認範囲は[受入確認](acceptance.md)へまとめています。

ネットワーク上の指定、本文の参照、共有リンク経由の対象についても、実行ファイルが判定不能として返すことを確認します。また、実行ファイルの直接・間接の依存に、通信機能の`net`やプロセス起動機能の`os/exec`が加わればテストを失敗させます。これらは通信がなかったことを実測する検査とは分けて扱います。

実際に割り当てたネットワークドライブは、`RULECHECK_TEST_NETWORK_DRIVES`へ`Y:;Z:`のように明示して追加確認できます。共有内には検証データを作らず、ルート・子パス・本文の参照・共有リンク・`PATH`が拒否されることを確認します。通常の必須検証ではこの実機確認だけをスキップします。実行方法と通信観測の手順は[受入確認](acceptance.md#割当ドライブの実機確認)を参照してください。

通信を観測するときは、製品と受入テストを[観測前にビルド](acceptance.md#通信観測前の準備)し、[scripts/run-observation.ps1](../scripts/run-observation.ps1)で準備済みのテストを起動します。`-CheckOnly`で成果物のSHA-256と指定ドライブの種別を事前に確認でき、本実行では固有の保存先へログを残し、スキップや必要な試験の未実行も失敗として扱います。観測中にGoのビルドや依存取得は行いません。管理者権限が必要な採取と、割当ドライブが見える通常利用者のテスト実行を分けます。Process Monitorの全イベントをPML原本とCSVで保持し、同じ試験のETLと照合する手順は[受入確認](acceptance.md#通信観測の手順)を参照してください。

開発時のGoのビルド・テストはキャッシュや実行ファイルを作成します。検査対象を変更しないという要件は、完成後の`rulecheck scan`の実行に適用します。成果物は`bin/`へ置き、ローカル作業用に`.cache/`を使った場合もGitの管理対象から除外します。書式検査はソースを置く`cmd/`と`internal/`の配下を再帰的に確認し、ビルド条件で除外されるファイルやパッケージも含めます。`.cache/`内の依存ソースは書式検査の対象に含めません。

## 構成と文書

| 場所 | 役割 |
| --- | --- |
| `cmd/rulecheck/` | 実行ファイルの入口とプロセス終了 |
| `internal/cli/` | 引数検証、使い方・版番号・通常結果・JSONの表示とテスト |
| `internal/report/` | 結果型、JSON形式の版番号、分類別件数、並び順、状態・終了コード |
| `internal/scan/` | ファイル・共有リンクの解決、同一実体の集約、Markdown参照、スキルの列挙・メタデータ・名前、CLI候補の検索 |
| `internal/version/version.go` | CLI版番号の正本 |
| `scripts/check.ps1` | 必須検証の入口 |
| `scripts/run-observation.ps1` | 観測用の準備済み成果物・割当ドライブの事前確認と受入試験、試験ごとのログ保存 |
| [docs/requirements.md](requirements.md) | 移動した要件定義書。検査範囲と受入条件の正本 |
| [docs/architecture.md](architecture.md) | 採用した構成、読み取り専用の境界、配布方針 |
| [docs/development-plan.md](development-plan.md) | 後続実装の順序、完了条件と現在地 |
| [docs/acceptance.md](acceptance.md) | 受入条件と実行ファイルのテストの対応、確認結果と未確認事項 |
| [docs/dependencies.md](dependencies.md) | 構文解析方式の選定、固定版、導入時の確認 |
| [AGENTS.md](../AGENTS.md) | このプロジェクトで作業するエージェントへの指示 |
| `scripts/package.ps1` | Windows x64用ZIP・SHA-256・Scoop定義の生成 |
| `bucket/rulecheck.json` | Scoopの配布一覧に登録する定義 |
