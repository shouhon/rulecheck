# rulecheck

LLM向けの指示ファイルとスキルについて、参照先・共有リンク・必要なCLIの存在を読み取り専用で検査する、Windows向けのコマンドラインツールです。

ファイルと共有リンク、Markdownのローカル参照、直下のスキルのYAMLメタデータ・名前、CLI候補の検索に対応しています。正常・注意のみなら終了コード`0`、エラーがあれば`1`、判定不能があれば`2`を返します。検査本体の実装工程M3は完了し、全体の受入確認M4を進めています。

## 開発環境

- Windows 11、PowerShell 7。
- Goの最低版は[go.mod](go.mod)の`go`指定を参照してください。初期設定はGo 1.27.3、Windows/amd64で確認しています。
- 実行時の外部CLIは不要です。構文解析にgoldmarkとgo-yamlを使い、Go標準のモジュール管理で`go.mod`・`go.sum`へ版とハッシュを固定しています。初回ビルドには依存の取得が必要です。選定と導入時の確認は[依存関係](docs/dependencies.md)を参照してください。
- 公開先が未指定のため、モジュール名は`rulecheck`です。ソースからのビルドで利用し、公開先を決めるまで`go install <公開先>@<版>`による導入は想定しません。

## 起動とビルド

プロジェクトのルートで実行します。

```powershell
go run ./cmd/rulecheck --help
go run ./cmd/rulecheck --version
go build -trimpath -o bin/rulecheck.exe ./cmd/rulecheck
./bin/rulecheck.exe --version
```

`bin/rulecheck.exe`が単独で動作する実行ファイルです。第三者ライブラリの表示は[THIRD_PARTY_NOTICES.txt](THIRD_PARTY_NOTICES.txt)へ保持しており、配布する際は実行ファイルと併せて同梱します。インストーラー、署名、自動配布は今回の対象に含みません。コマンドの終了コードを確かめる場合は、`go run`ではなくビルドした実行ファイルと`$LASTEXITCODE`を使ってください。

## scanの指定と出力

```powershell
./bin/rulecheck.exe scan --rules ./AGENTS.md --skills ./skills --command rg --json
$LASTEXITCODE
```

`--rules`・`--skills`・`--command`は繰り返して指定でき、少なくとも1つの対象が必要です。`--rules=./AGENTS.md`のような指定にも対応します。空白を含むパスは引用符で囲み、`-`で始まる値は`=`で指定してください。`--command`にはパスではなく、`PATH`から探す名前を指定します。

通常の結果は標準出力へ日本語で表示します。`--json`指定時は標準出力へJSONオブジェクト1件だけを出します。不正引数は終了コード`64`となり、標準エラーへ診断を出します。この場合も`--json`があれば、標準出力には`status: invalid_arguments`のJSONを返します。指定順を変えても結果の順序は一定です。同じ種類で同じ実体を指定した対象はまとめ、相対・絶対パス、シンボリックリンク、ハードリンク、ジャンクションによる別名と参照関係を残します。

指定ファイルの不存在、リンク切れ・循環、対象の種類の誤りはエラー、読取不能は判定不能として区別します。本文はUTF-8で読み、解釈できない文字コードやNULを含むものは判定不能とします。`--skills`は親の直下にある各ディレクトリの`SKILL.md`だけを列挙し、補助ディレクトリの中へ再帰しません。UNCパス、ネットワークドライブ、デバイス名などは開かず判定不能とします。出力項目と検査範囲の表現は[構成と実装方針](docs/architecture.md#結果の形式)を参照してください。

MarkdownはCommonMarkの通常・参照形式・画像のリンクを対象とします。相対参照は記述元の実体ディレクトリを基準にし、参照先の存在と種類だけを確認します。空白を含む参照は`[説明](<空白 日本語.md>)`または`%20`を使い、Windowsの絶対パスも指定できます。外部URL、見出し、コード中の例は対象外として残します。未定義の参照形式、二重角括弧、参照属性を含むHTML、`file:` URIなどの未対応表記は判定不能です。通常表示とJSONには、異常の記述元・分かる場合の行番号・関係するパスを残し、本文や外部URL自体は転載しません。

スキルは先頭の`---`で囲んだYAMLを解析し、`name`・`description`の欠落、空文字列、型の誤り、不正な構文をエラーにします。引用文字列、複数行文字列、別名、マージ、追加項目に対応します。同じ実体への別名をまとめ、異なる実体での同名と、実体ディレクトリ名との差異を注意として表示します。YAMLの値自体は診断へ転載しません。

`--command`は現在の`PATH`を指定順に調べ、最初に確認できた通常ファイルの候補と共有リンクの実体を表示します。Windowsでは`PATHEXT`の順序を使い、空なら`.COM;.EXE;.BAT;.CMD`を使います。拡張子を含む名前は、その名前も先に確認します。作業ディレクトリは暗黙に追加せず、`PATH`に明記された相対ディレクトリだけを作業ディレクトリ基準で解決します。ネットワーク上や確認できない場所を除いて候補が見つかれば存在確認を完了し、候補がなく確認できない場所が残れば、不足と断定せず判定不能とします。候補の本文は読まず、起動・認証・実行権限・実行ポリシーは確認しません。PowerShellの関数とエイリアスも対象外です。

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

書式の修正は`go fmt ./...`で行います。テストはCLIの契約に加え、一時ディレクトリ内でファイルの種類・読取可否、共有リンクと別名、循環・リンク切れ、直下だけのスキル列挙、Markdownの構文と相対参照の基準、YAMLの型・欠落・同名の区別を確認します。CLI候補は模擬の`PATH`と`PATHEXT`を使い、検索順序、候補の不足と判定不能、本文の非読取と非実行を確認します。Windowsでは読取を拒むファイルハンドルを使い、参照先の本文を開かないこと、特殊なパス、ジャンクションも確認します。リンクを作成できない場合は該当テストを未確認としてスキップするため、`go test -v ./internal/scan`で理由を確認してください。製品全体の受入確認の現在地は[開発計画](docs/development-plan.md)に残します。

開発時のGoのビルド・テストはキャッシュや実行ファイルを作成します。検査対象を変更しないという要件は、完成後の`rulecheck scan`の実行に適用します。成果物は`bin/`へ置き、ローカル作業用に`.cache/`を使った場合もGitの管理対象から除外します。

## 構成と文書

| 場所 | 役割 |
| --- | --- |
| `cmd/rulecheck/` | 実行ファイルの入口とプロセス終了 |
| `internal/cli/` | 引数検証、使い方・版番号・通常結果・JSONの表示とテスト |
| `internal/report/` | 結果型、JSON形式の版番号、分類別件数、並び順、状態・終了コード |
| `internal/scan/` | ファイル・共有リンクの解決、同一実体の集約、Markdown参照、スキルの列挙・メタデータ・名前、CLI候補の検索 |
| `internal/version/version.go` | CLI版番号の正本 |
| `scripts/check.ps1` | 必須検証の入口 |
| [docs/requirements.md](docs/requirements.md) | 移動した要件定義書。検査範囲と受入条件の正本 |
| [docs/architecture.md](docs/architecture.md) | 採用した構成、読み取り専用の境界、未決定事項 |
| [docs/development-plan.md](docs/development-plan.md) | 後続実装の順序、完了条件と現在地 |
| [docs/dependencies.md](docs/dependencies.md) | 構文解析方式の選定、固定版、導入時の確認 |
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
