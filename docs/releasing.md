# リリース手順

公開先は[GitHub](https://github.com/shouhon/rulecheck)と[GitHub Releases](https://github.com/shouhon/rulecheck/releases)です。Windows 11 x64用のZIPを配布し、同じリポジトリの`bucket/rulecheck.json`でScoopから導入できるようにします。実行ファイルは未署名です。

## mainへのpushによる自動公開

[GitHub Actions](../.github/workflows/ci.yml)はmainへのpushとmainを対象とするプルリクエストで、Windows上の必須検証を実行します。mainへのpushでは、push前のコミットとpush後のコミットの`internal/version/version.go`の`Current`を比較し、番号が変わっている場合だけ検証成功後に公開します。複数コミットをまとめてpushしても、最後の1コミットだけでなくpush全体を比較します。番号の据え置きとプルリクエストでは公開せず、番号が下がった場合や比較元を取得できない場合は失敗として停止します。

公開処理は既存の[scripts/package.ps1](../scripts/package.ps1)でWindows x64のZIP・`checksums.txt`・`rulecheck.json`を生成し、push後のコミットを指定して`v<版番号>`のタグと下書きReleaseを作ります。添付ファイル3件をダウンロードし、生成物とSHA-256が一致した場合だけ公開します。説明はGitHubの自動生成を使います。`docs/release-notes.md`は手動公開用です。利用者向けの変更はコミットやプルリクエストの題名にも記載してください。

同じ版の実行は同時に公開しません。異なる版の公開完了順が逆転しても、LatestはGitHubの[`make_latest=legacy`](https://docs.github.com/en/rest/releases/releases#update-a-release)で版番号と作成日時から選びます。公開済みの同じ版・同じコミットは変更せず終了し、別コミットのタグやReleaseは失敗として停止します。タグはGitHub上で確認し、存在しない場合だけ作成します。作成競合は失敗として停止し、公開直前にもタグの対象を再確認します。途中で失敗した場合はActionsでその実行を再実行します。同じコミットの下書きがあれば不足する添付ファイルだけを送信し、既存分も含めて再照合します。不一致の添付ファイルは自動で差し替えません。下書きと生成物を確認し、必要なら新しい版で公開してください。

GitHubの一時的な`GITHUB_TOKEN`を使い、追加の秘密情報の登録は不要です。`contents: write`は公開ジョブだけに与え、ソース取得時の認証情報は保存しません。通常の検証とプルリクエストは読み取り権限で実行します。リポジトリや組織でActionsが制限されている場合は、GitHub公式の`actions/checkout`・`actions/setup-go`と公開ジョブの書き込み権限を許可してください。使用するActionはコミットのSHAへ固定しています。

この処理はReleaseへの添付までを自動化します。Scoopの配布一覧`bucket/rulecheck.json`の更新は以下の準備手順で行い、版番号の更新と一緒に反映してください。自動処理からmainへのコミットやpushは行いません。

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

## 手動でGitHubへ公開

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
