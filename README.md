# rulecheck

[![GitHub Release](https://img.shields.io/github/v/release/shouhon/rulecheck)](https://github.com/shouhon/rulecheck/releases/latest)
[![ライセンス: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

LLM向けの指示ファイルとスキルの参照切れ、壊れた共有リンク、必要なCLIの不足をまとめて見つける、Windows用のコマンドラインツールです。

指定した対象を読み取り専用で検査します。ファイルや設定の変更、検査対象のコマンドの実行、外部URLへの接続は行いません。

## 主な機能

- 指示ファイルと共有リンクの存在、リンク切れ・循環・同じ実体への別名を確認。
- Markdownのローカル参照について、参照先の存在と種類を確認。
- 直下のスキルの`SKILL.md`について、YAMLの`name`・`description`、名前の重複・ディレクトリ名との差異を確認。
- 指定したCLI名を`PATH`から検索し、候補と共有リンクの実体を確認。
- 日本語の結果表示とJSON出力に対応。終了コードを使って自動処理へ組み込み可能。

## インストール

対応環境はWindows 11 x64です。実行時にGoや追加のCLIは必要ありません。

### Scoop

[Scoop](https://scoop.sh/)を導入したPowerShellで実行してください。このリポジトリを配布一覧として追加します。

```powershell
scoop bucket add rulecheck https://github.com/shouhon/rulecheck
scoop install rulecheck/rulecheck
rulecheck --version
```

更新と削除:

```powershell
scoop update
scoop update rulecheck
scoop uninstall rulecheck
```

### ZIPをダウンロード

1. [GitHub Releases](https://github.com/shouhon/rulecheck/releases/latest)から`rulecheck_<版番号>_windows_amd64.zip`と`checksums.txt`をダウンロードします。
2. `Get-FileHash ./rulecheck_<版番号>_windows_amd64.zip -Algorithm SHA256`の値を`checksums.txt`と照合します。
3. ZIPを展開し、`rulecheck.exe`を実行します。必要なら展開先を`PATH`へ追加してください。

ZIPにはMITライセンスと第三者ライブラリのライセンス表示を同梱します。実行ファイルは未署名です。Windowsの警告や組織の制限で実行できない場合は、管理者の方針に従ってください。

### Goからインストール

[go.mod](go.mod)で指定する版以上のGoを使い、Windows上で実行します。

```powershell
go install github.com/shouhon/rulecheck/cmd/rulecheck@latest
```

Goの実行ファイルの保存先（既定は`$HOME/go/bin`）を`PATH`へ追加してください。

## 使い方

```powershell
# 指示ファイルを検査
rulecheck scan --rules ./AGENTS.md

# 指示ファイル・スキル・必要なCLIをまとめて検査
rulecheck scan --rules ./AGENTS.md --skills ./skills --command rg --command git

# 結果をJSONで取得
rulecheck scan --rules ./AGENTS.md --json
$LASTEXITCODE

# 対象外の詳細を省き、種類別の件数だけを取得
rulecheck scan --rules ./AGENTS.md --json --summary

# 正常項目を省き、検査対象数・問題・件数を表示
rulecheck scan --skills ./skills --quiet

# 1件のスキルの構成と参照を検査
rulecheck scan --skill ./skills/new-skill --quiet

rulecheck --help
```

`--rules`・`--skills`・`--skill`・`--command`は繰り返し指定でき、少なくとも1つの対象が必要です。空白を含むパスは引用符で囲んでください。`--skills`は指定した親ディレクトリの直下だけを調べます。`--skill`は指定先の`SKILL.md`を調べ、親や兄弟スキルを列挙しません。JSONは標準出力へオブジェクト1件として出力します。

`--summary`は通常表示とJSONの両方で使えます。対象外の診断を種類別の件数にまとめ、エラー・注意・判定不能の詳細は残します。対象外の詳細が必要な場合は`--summary`を外してください。

`--quiet`はさらに正常項目のパスや別名を省略し、検査対象数と問題の詳細を表示します。対象外だけの項目も件数にまとめます。通常表示とJSONの両方に対応し、`--summary`との併用も可能です。どちらの表示オプションも検査内容と終了コードは変えません。

| 終了コード | 意味 |
| --- | --- |
| `0` | 正常、または注意のみ |
| `1` | エラーあり |
| `2` | 読取不能・未対応などで判定不能あり。エラーもある場合はこちらを優先 |
| `64` | 引数が不正 |

指定したファイルの本文はUTF-8を前提とします。参照先の本文は再帰的に読みません。外部URL・見出し・コード中の例は参照検査の対象外です。UNCパスやネットワークドライブは開かず、判定不能とします。自然文の意味の矛盾、各製品の指示の読み込み順序、CLIの起動可否や認証状態は検査しません。詳しくは[使い方の詳細](docs/usage.md)を参照してください。

## 開発

Windows 11・PowerShell 7と、[go.mod](go.mod)で指定する版以上のGoを使います。

```powershell
git clone https://github.com/shouhon/rulecheck.git
cd rulecheck
go build -trimpath -o bin/rulecheck.exe ./cmd/rulecheck
./bin/rulecheck.exe --version

# 書式・モジュール整合性・静的検査・テスト・ビルド・起動を確認
pwsh -NoProfile -File ./scripts/check.ps1
```

変更前に[AGENTS.md](AGENTS.md)を確認してください。不具合の報告や改善提案は[Issues](https://github.com/shouhon/rulecheck/issues)へお願いします。報告に個人の設定本文や秘密情報を含めないでください。

| 文書 | 内容 |
| --- | --- |
| [使い方の詳細](docs/usage.md) | 引数、参照・スキル・CLIの検査範囲、判定の扱い |
| [開発と検証](docs/development.md) | 必須検証、模擬データによるテスト、ソースの構成 |
| [リリース手順](docs/releasing.md) | ZIP・Scoopの配布、公開、旧版への復旧 |
| [要件定義書](docs/requirements.md) | 製品の範囲と受入条件の正本 |
| [構成と実装方針](docs/architecture.md) | 内部構成、読み取り専用の境界、JSON形式 |
| [開発計画](docs/development-plan.md) | 工程と実装状況 |
| [受入確認](docs/acceptance.md) | 要件との対応、Windowsでの確認結果と限界 |
| [依存関係](docs/dependencies.md) | 構文解析ライブラリの選定と固定版 |

## 版管理

- 配布単位は`rulecheck`実行ファイル1つです。番号の正本は[internal/version/version.go](internal/version/version.go)の`Current`で、`--version`もそこから表示します。
- `メジャー.マイナー.パッチ`を使います。互換性の対象はコマンド・引数、終了コード、JSONの構造と機械判定用の`code`です。正式版の互換性を壊す変更はメジャー、互換性を保つ機能追加はマイナー、修正はパッチを更新します。`0.x`では、互換性を壊す変更と機能追加はマイナー、修正はパッチを更新します。
- 初版の検査機能が完成するまでは未配布の初回番号を据え置きます。その後は、動作・出力・配布内容を変える一まとまりの作業が完了したときに一度更新します。開発者向け文書・テストだけの変更では据え置きます。
- 作業開始時に正本を確認し、完了時に必須検証でビルドした`bin/rulecheck.exe --version`と照合します。配布スクリプトは同じ正本と実行ファイルの表示を照合し、ZIP名・Scoopの定義・タグへ反映します。公開済みの同じ番号を別内容に差し替えません。
- 番号を変えたら、配布物から生成した`rulecheck.json`を`bucket/rulecheck.json`へ反映して同じコミットに含めます。必須検証は定義の版番号・配布URL・SHA-256の形式も確認します。手順は[リリース手順](docs/releasing.md#準備と検証)を参照してください。
- JSONの`schema_version`はCLIの版と別に扱い、正本は[internal/report/report.go](internal/report/report.go)の`SchemaVersion`です。タグ・公開・署名・配布は、対象と操作が明示的に認可された場合に行います。

## ライセンス

[MIT License](LICENSE)。第三者ライブラリのライセンスと著作権表示は[THIRD_PARTY_NOTICES.txt](THIRD_PARTY_NOTICES.txt)を参照してください。
