# rulecheck

LLM向けの指示ファイルとスキルについて、参照先・共有リンク・必要なCLIの存在を読み取り専用で検査する、Windows向けのコマンドラインツールです。

現在は開発基盤と、`scan`の引数処理・結果の分類・通常表示・JSON出力まで完了しています。ファイル・リンク・スキル・CLIの検査本体は未実装です。有効な対象を指定した`scan`は、対象ごとに未実装を「判定不能」として報告し、終了コード`2`を返します。検査完了とは表示しません。

## 開発環境

- Windows 11、PowerShell 7。
- Goの最低版は[go.mod](go.mod)の`go`指定を参照してください。初期設定はGo 1.27.3、Windows/amd64で確認しています。
- 実行時の外部CLIや外部ライブラリへの依存はありません。Go標準のモジュール管理を使います。外部モジュールがないため、`go.sum`はありません。
- 公開先が未指定のため、モジュール名は`rulecheck`です。ソースからのビルドで利用し、公開先を決めるまで`go install <公開先>@<版>`による導入は想定しません。

## 起動とビルド

プロジェクトのルートで実行します。

```powershell
go run ./cmd/rulecheck --help
go run ./cmd/rulecheck --version
go build -trimpath -o bin/rulecheck.exe ./cmd/rulecheck
./bin/rulecheck.exe --version
```

`bin/rulecheck.exe`が単独で動作する実行ファイルです。インストーラー、署名、自動配布は今回の対象に含みません。コマンドの終了コードを確かめる場合は、`go run`ではなくビルドした実行ファイルと`$LASTEXITCODE`を使ってください。

## scanの指定と出力

```powershell
./bin/rulecheck.exe scan --rules ./AGENTS.md --skills ./skills --command rg --json
$LASTEXITCODE
```

`--rules`・`--skills`・`--command`は繰り返して指定でき、少なくとも1つの対象が必要です。`--rules=./AGENTS.md`のような指定にも対応します。空白を含むパスは引用符で囲み、`-`で始まる値は`=`で指定してください。`--command`にはパスではなく、`PATH`から探す名前を指定します。

通常の結果は標準出力へ日本語で表示します。`--json`指定時は標準出力へJSONオブジェクト1件だけを出します。不正引数は終了コード`64`となり、標準エラーへ診断を出します。この場合も`--json`があれば、標準出力には`status: invalid_arguments`のJSONを返します。指定順を変えても結果の順序は一定で、完全に同じ種類・文字列の対象を繰り返した場合は1件にまとめます。実体の同一性を調べる処理は未実装です。

現段階では指定されたファイルを読み取らず、存在も確認しません。存在しない対象も判定不能となります。出力項目と検査範囲の表現は[構成と実装方針](docs/architecture.md#結果の形式)を参照してください。

## 必須検証

次の1コマンドを検証の入口とします。どの作業ディレクトリから呼び出しても、スクリプト自身がプロジェクトのルートへ移動します。

```powershell
pwsh -NoProfile -File ./scripts/check.ps1
```

[scripts/check.ps1](scripts/check.ps1)は次を順番に実行し、失敗時に停止します。

| 確認 | コマンド |
| --- | --- |
| 書式 | `gofmt -l .`。出力があれば失敗 |
| モジュールの整合性 | `go mod tidy -diff`。定義ファイルは変更しない |
| 静的検査 | `go vet ./...` |
| テスト | `go test ./...` |
| ビルド | `go build -trimpath -o bin/rulecheck.exe ./cmd/rulecheck` |
| 起動 | `./bin/rulecheck.exe --version` |

書式の修正は`go fmt ./...`で行います。現時点のテストはCLIの入口、引数検証、JSONと通常表示、結果の並び順、分類別件数と終了コードの優先順位、出力失敗を確認します。検査本体について要件の受入条件全体を満たすテストではありません。

開発時のGoのビルド・テストはキャッシュや実行ファイルを作成します。検査対象を変更しないという要件は、完成後の`rulecheck scan`の実行に適用します。成果物は`bin/`へ置き、ローカル作業用に`.cache/`を使った場合もGitの管理対象から除外します。

## 構成と文書

| 場所 | 役割 |
| --- | --- |
| `cmd/rulecheck/` | 実行ファイルの入口とプロセス終了 |
| `internal/cli/` | 引数検証、使い方・版番号・通常結果・JSONの表示とテスト |
| `internal/report/` | 結果型、JSON形式の版番号、分類別件数、並び順、状態・終了コード |
| `internal/version/version.go` | CLI版番号の正本 |
| `scripts/check.ps1` | 必須検証の入口 |
| [docs/requirements.md](docs/requirements.md) | 移動した要件定義書。検査範囲と受入条件の正本 |
| [docs/architecture.md](docs/architecture.md) | 採用した構成、読み取り専用の境界、未決定事項 |
| [docs/development-plan.md](docs/development-plan.md) | 後続実装の順序、完了条件と現在地 |
| [AGENTS.md](AGENTS.md) | このプロジェクトで作業するエージェントへの指示 |

## 版管理

- 配布単位は`rulecheck`実行ファイル1つです。番号の正本は[internal/version/version.go](internal/version/version.go)の`Current`だけとし、`--version`もそこから表示します。初回番号は要件定義書8節に従います。
- 正式版は`メジャー.マイナー.パッチ`を使います。互換性の対象はコマンド・引数、終了コード、JSONの構造と機械判定用の`code`です。正式版の互換性を壊す変更はメジャー、互換性を保つ機能追加はマイナー、修正はパッチを更新します。公開前の`0.x`では、互換性を壊す変更と機能追加はマイナー、修正はパッチを更新します。
- 初版の検査機能が完成するまでは、未配布の初回番号を据え置き、実装状況を[開発計画](docs/development-plan.md)に記録します。その後は、動作・出力・配布内容を変える一まとまりの作業が完了したときに一度更新します。開発者向け文書・テストだけの変更では据え置きます。
- 作業開始時に正本を確認し、完了時に変更内容と更新規則を照合します。検証スクリプトで再ビルドし、`--version`の結果が正本と一致することを確認します。今はローカル成果物名を`bin/rulecheck.exe`に固定します。配布を始める際は、この実行ファイルの`--version`から取得した番号を配布物名へ反映し、公開済みの同じ番号を別内容に差し替えません。
- JSONの`schema_version`はCLIの版と別に扱います。正本は[internal/report/report.go](internal/report/report.go)の`SchemaVersion`で、初回値は要件定義書に従う`1`です。Gitのコミット識別子でソースを識別します。タグ・公開・署名・配布は別途認可されたときに実施します。

## 参照した公式資料

- [Goモジュールの構成](https://go.dev/doc/modules/layout)
- [go.modの指定](https://go.dev/doc/modules/gomod-ref)
- [Goコマンド](https://pkg.go.dev/cmd/go)
- [gofmt](https://pkg.go.dev/cmd/gofmt)
