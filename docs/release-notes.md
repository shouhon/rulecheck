# rulecheck v0.1.2

GitHub Releasesでの初回配布です。Windows 11 x64向けのZIPと、Scoopによる導入に対応しました。

## 主な機能

- 指示ファイルと共有リンクの存在、リンク切れ・循環・別名を確認します。
- Markdownのローカル参照、スキルのYAMLメタデータと名前を検査します。
- 必要なCLIを`PATH`から探し、候補を実行せずに存在を確認します。
- 日本語の通常表示、JSON、処理結果に応じた終了コードを使えます。

## この版での変更

- 配布先を`shouhon/rulecheck`とGitHub Releasesに統一しました。
- Scoopの配布一覧、ZIP・SHA-256・Scoop定義の生成手順を追加しました。
- Goのモジュール名を`github.com/shouhon/rulecheck`へ変更し、`go install`による導入に対応しました。
- READMEを整理し、MITライセンス（Copyright (c) 2026 shouhon）を採用しました。
- CLI版を`0.1.1`から`0.1.2`へ更新しました。JSON形式の版は`1`を維持しています。

## インストール

[Scoop](https://scoop.sh/)導入済みのPowerShellで実行します。

```powershell
scoop bucket add rulecheck https://github.com/shouhon/rulecheck
scoop install rulecheck/rulecheck
rulecheck --version
```

ZIPの場合は`rulecheck_0.1.2_windows_amd64.zip`を取得し、`checksums.txt`のSHA-256と照合して展開してください。ZIPには`rulecheck.exe`、`LICENSE`、`THIRD_PARTY_NOTICES.txt`を同梱しています。実行ファイルは未署名です。

## 検証と制約

- Windows/amd64・Go 1.27.3・PowerShell 7で、必須検証（書式・モジュール整合性・静的検査・全テスト・ビルド・起動）に成功しました。CIは未設定です。
- ZIP内のライセンス表示、版番号、使い方、模擬データによる正常・不存在・不正引数、JSON形式、対象の非変更・候補CLIの非実行を確認しました。
- 同じツールチェーンと固定済み依存を使い、別々の出力先で生成したZIPのSHA-256が一致しました。異なる環境間の再現性は未検証です。
- 今回の通常検証では割当ネットワークドライブの実機試験をスキップしました。過去の受入結果と通信観測の範囲は[受入確認](https://github.com/shouhon/rulecheck/blob/v0.1.2/docs/acceptance.md)を参照してください。共有リンクの検証は成功しています。
- Windows x64以外は配布・動作確認の対象外です。外部URL、自然文の意味、CLIの起動可否・認証状態は検査しません。
