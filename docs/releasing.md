# リリース手順

公開先は[GitHub](https://github.com/shouhon/rulecheck)と[GitHub Releases](https://github.com/shouhon/rulecheck/releases)です。Windows 11 x64用のZIPを配布し、同じリポジトリの`bucket/rulecheck.json`でScoopから導入できるようにします。実行ファイルは未署名です。

## 準備と検証

1. [READMEの版管理](../README.md#版管理)に従って`internal/version/version.go`を更新します。既に今回の変更で更新済みなら重ねて増やしません。
2. Windows x64・PowerShell 7で必須検証を実行します。

   ```powershell
   pwsh -NoProfile -File ./scripts/check.ps1
   ```

3. 配布物を生成します。このスクリプトは公開・Git操作・依存の更新を行いません。Goの依存は事前に取得しておきます。

   ```powershell
   pwsh -NoProfile -File ./scripts/package.ps1
   $version = (./bin/rulecheck.exe --version) -replace '^rulecheck ', ''
   $releaseDirectory = "bin/release/$version"
   ```

   生成先は`bin/release/<版番号>/`です。ZIPの内容は`rulecheck.exe`・`LICENSE`・`THIRD_PARTY_NOTICES.txt`だけです。ZIPに加えて、SHA-256の一覧`checksums.txt`とScoopの定義`rulecheck.json`を生成します。同じ出力先にZIPがある場合は上書きせず失敗します。再生成時は`-OutputDirectory bin/release-check`のように別の保存先を指定してください。

4. ZIPを一時ディレクトリへ展開し、`--version`・`--help`と模擬データの`scan --json`を確認します。JSONの版、終了コード、前後の対象ファイルの一致を確認します。ライセンス2件がソースと一致すること、ZIPとScoopの定義のSHA-256が`checksums.txt`と一致することも確認します。
5. 生成した定義を配布一覧へ反映します。

   ```powershell
   New-Item -ItemType Directory -Path bucket -Force | Out-Null
   Copy-Item -LiteralPath "$releaseDirectory/rulecheck.json" -Destination bucket/rulecheck.json
   git diff --check
   ```

6. 別の出力先でも生成し、ZIPのSHA-256が一致することを確認します。同じGo・PowerShell/.NET・固定済み依存・環境変数での再現確認です。異なるツールチェーンやOSでの一致を保証するものではありません。ソースの絶対パスとGitの作業ツリー状態は実行ファイルへ埋め込まず、ZIP内の日時は固定しています。
7. 差分に秘密情報・個人用設定・生成物がないことを確認し、必要な検証を通した変更だけをコミットします。`bin/`はGitへ追加しません。変更後に配布物を作り直した場合は、配布一覧と検証結果も対応するものへ更新します。

## GitHubへ公開

ユーザーから対象と公開操作の明示的な認可を得た場合に実行します。ブランチの保護や認証・セキュリティの制限を回避しません。失敗した操作は結果を照会し、未完了と確認できた場合だけ再実行します。

1. `gh`の認証、公開先、保護設定、既存タグ・Releaseを確認します。必要なら利用者自身が`gh auth login --hostname github.com`を実行します。通常の変更は設定済みの追跡先へプッシュします。
2. `gh run list --commit <コミット>`で既存の必須CIがあれば成功を確認します。CIが未設定の場合は、手元の必須検証の結果と確認環境をReleaseへ記載します。
3. 作業ツリーが検証したコミットと一致することを確認し、公開先の同じコミットを対象にタグと下書きReleaseを作ります。以下のファイルパスはプロジェクトのルートを基準とします。

   ```powershell
   $commit = git rev-parse HEAD
   gh release create "v$version" "$releaseDirectory/rulecheck_${version}_windows_amd64.zip" "$releaseDirectory/checksums.txt" "$releaseDirectory/rulecheck.json" --repo shouhon/rulecheck --target $commit --title "rulecheck v$version" --notes-file docs/release-notes.md --draft
   ```

   `docs/release-notes.md`へ利用者向けの変更、導入方法、確認環境、未確認事項をまとめます。`gh release create --target`によって対象コミットのタグもGitHub上に作成されます。公開済みのタグや配布物を同じ版番号で置き換えません。

4. 下書きの添付ファイルを別の一時領域へダウンロードし、生成物とハッシュが一致することを確認します。ファイル名・版番号・MITの表示も確認して公開します。

   ```powershell
   gh release view "v$version" --repo shouhon/rulecheck --json tagName,isDraft,targetCommitish,assets
   gh release edit "v$version" --repo shouhon/rulecheck --draft=false --latest
   ```

5. 公開後にReleaseのURL・タグの対象コミット・添付ファイルを再確認します。READMEのScoopコマンドを、既存の個人設定とは分けた検証環境で実行し、版番号と`scan`を確認します。`go install`も別の`GOBIN`へ導入して確認します。

Scoopの配布一覧の構成は[公式のBuckets](https://github.com/ScoopInstaller/Scoop/wiki/Buckets)、定義形式は[App Manifests](https://github.com/ScoopInstaller/Scoop/wiki/App-Manifests)、公開操作は[`gh release create`](https://cli.github.com/manual/gh_release_create)を参照してください。

## 旧版へ戻す

- ZIPで利用している場合は[過去のReleases](https://github.com/shouhon/rulecheck/releases)から旧版のZIPと`checksums.txt`を取得し、照合して別のディレクトリへ展開します。旧版の実行ファイルを直接呼ぶか、`PATH`の参照先をそのディレクトリへ変えます。
- Scoopの場合、導入済みの旧版が残っていれば`scoop reset rulecheck@<旧版番号>`で切り替えます。旧版がなければ、過去のReleaseから旧版の`rulecheck.json`をダウンロードし、`scoop uninstall rulecheck`の後に`scoop install ./rulecheck.json`で導入します。
- rulecheckは検査対象や設定を書き換えません。旧版へ戻すためのデータ移行は不要です。不具合のある公開版は差し替えず、別のパッチ版で修正します。初回公開には戻せる過去のReleaseはありません。
