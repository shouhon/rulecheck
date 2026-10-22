# 受入確認

## 対象と判定

[要件定義書7節](requirements.md#7-受入条件)と、実行ファイルを使った確認の対応を記録します。検査範囲の正本は要件定義書です。

対象は基準コミット`da88587`に、観測前に準備した実行ファイルを使う受入確認と、その手順を追加した状態です。CLI版は`0.1.1`、JSON形式は`1`です。今回はテストと開発文書だけを変更しています。全体の受入判定は、後述の通信監視が未完了のため保留します。

## 再実行

必須検証の`go test ./...`から、Windows用の[TestExecutableAcceptance](../cmd/rulecheck/acceptance_windows_test.go)も実行します。受入確認だけをキャッシュを使わず再実行し、対象の実行ファイルのSHA-256とスキップの有無を確認する場合は、プロジェクトのルートで次を使います。

```powershell
go test -count=1 -v ./cmd/rulecheck
```

テスト内で`go build -trimpath`により一時ディレクトリへ実行ファイルを作り、別プロセスとして起動します。検査中の`PATH`・`PATHEXT`、利用者の設定先と一時ファイルの保存先は模擬ディレクトリへ向けます。ビルド時はプロジェクトの既存のGo設定を使います。実際の個人用設定や外部サービスは検査しません。

通信の観測時は、`RULECHECK_TEST_BINARY`へ準備済みの`rulecheck.exe`の絶対パスを指定すると、受入テスト内のビルドを省けます。その場合もSHA-256を記録し、版番号と既存の受入条件を照合します。不正なパス、ファイルの読取失敗、版番号の不一致は失敗とし、別の実行ファイルをビルドして代用しません。テスト本体も事前に実行ファイルへまとめ、観測中にGoコマンドを起動しない手順を下記へ示します。

共有リンクや再解析ポイントを作成できなければ、必要な項目を未確認としてスキップします。終了コードが成功でも、スキップした受入条件を確認済みとは扱いません。

## 割当ドライブの実機確認

[TestExecutableMappedDrives](../cmd/rulecheck/mapped_drives_windows_test.go)は、明示した割当ドライブだけを使う追加の受入試験です。割当を行った利用者の通常のPowerShellから、プロジェクトのルートで実行します。

```powershell
$env:RULECHECK_TEST_NETWORK_DRIVES = 'Y:;Z:'
try {
    go test -count=1 -v ./cmd/rulecheck -run '^TestExecutableMappedDrives$'
    if ($LASTEXITCODE -ne 0) { throw '割当ドライブの受入確認に失敗しました。' }
}
finally {
    Remove-Item Env:RULECHECK_TEST_NETWORK_DRIVES
}
```

既存のGo設定と固定済みモジュールを使い、新たな取得は不要です。指定はドライブ名だけに制限し、`GetDriveTypeW`がネットワークドライブを表す`4`を返すことを前提にします。未指定ならこの追加試験だけをスキップし、明示したドライブが見えない場合やローカルドライブの場合は失敗します。スキップによる確認済みの誤認を避けます。

各ドライブについて、ルート・子パス・スラッシュ区切り・拡張長パスを`--rules`と`--skills`へ指定します。通常・参照形式・画像の本文の参照、直接の`PATH`、UNCの試験と共通の共有リンク7経路も確認します。1ドライブ当たり13項目で、JSON・通常表示・終了コード2とローカルの模擬データの前後一致を検査します。

模擬ファイル・共有リンク・実行ファイルはすべてローカルの一時ディレクトリに作ります。共有側の子パスは名前を組み立てるだけで、試験側から作成・存在確認・列挙は行いません。リンクも種類を明示して作り、作成時に共有先の属性を調べません。共有内の既存ファイルは検査用データにせず、前後比較のためにも読みません。この試験は拒否の結果を確認するもので、共有へのアクセスや通信がなかったことの実測は別途必要です。

管理者への昇格やサンドボックスによって、通常の利用者が割り当てたドライブが見えなくなる場合があります。試験のために再割当や資格情報の変更は行わず、割当が見える通常の環境から実行してください。今回もサンドボックス内では種別`1`、サンドボックス外の非管理者環境ではY:・Z:とも種別`4`でした。

## 要件との対応

下表の項目名は`TestExecutableAcceptance`の子テストです。検査処理を直接呼ぶ既存テストと分け、実際の標準出力・標準エラー・終了コードから確認しています。

| 受入条件 | 実行ファイルでの確認項目 | 観測内容 |
| --- | --- | --- |
| 1 | 正常な共有リンクと実体基準の参照 | 実体1件へ集約し、実体パス・2つの別名・共有リンクを保持 |
| 2 | リンク切れと循環と読取不能／未対応の再解析ポイントを判定不能にする | `link_broken`・`link_cycle`・`path_unreadable`を区別。未対応の再解析ポイントも不存在や成功と誤認せず、終了コード2を返す。エラーとの併存時も判定不能を優先 |
| 3 | 正常な共有リンクと実体基準の参照／コードと外部参照と本文の指示を実行しない | 通常・参照形式・画像を解決し、外部参照・見出し・コードの対象外診断を保持 |
| 4 | 正常な共有リンクと実体基準の参照 | 別ディレクトリの共有リンクからも実体基準で解決し、空白・日本語・パーセント符号化に対応 |
| 5 | メタデータと直下だけの列挙 | 構文不正、必須項目の欠落と型の誤りを区別。追加項目は許容し、入れ子の`SKILL.md`は除外 |
| 6 | 同名の異なる実体と同一実体の別名 | 同一実体の別名では注意0件、異なる実体の同名では注意2件。終了コードはどちらも0 |
| 7 | 参照エラーとCLI不足と未対応表記／コードと外部参照と本文の指示を実行しない | 明示したCLI不足を検出し、本文やコード中の単語からCLIを追加しない |
| 8 | 正常な共有リンクと実体基準の参照／コードと外部参照と本文の指示を実行しない／メタデータと直下だけの列挙／未対応の再解析ポイントを判定不能にする | 本文の読取を拒む参照先とCLI候補でも存在確認が成功。本文・YAML・再解析データに埋めた模擬機密値は出力されず、模擬CLIの起動痕跡も作られない |
| 9 | 全子テスト／不正引数のJSONと出力先 | 正常・注意・エラー・判定不能・不正引数の状態、分類別件数、終了コード0・1・2・64と出力先が一致。対象の状態は前後で一致 |

診断の件数は、受入テストで個々の分類から数え直し、JSONの集計および通常表示と照合します。引数の順序・同じ対象の再指定を変えた場合は、JSONと通常表示の両方を比較します。参照エラーの記述元・行番号・関係するパス、未対応表記が参照検査だけを未完了にすることも確認します。

模擬データの前後比較は、通常ファイルのSHA-256・更新日時・属性、共有リンクを含む再解析データのSHA-256、ディレクトリの構成・更新日時・属性を対象にします。再解析データは`FILE_FLAG_OPEN_REPARSE_POINT`で対象自体を開き、参照先を辿らずに取得します。再解析ディレクトリの内部は列挙しません。読取に伴う最終アクセス日時は比較しません。作成直後のディレクトリ時刻の遅延反映を避け、通常のディレクトリ時刻を検査前に固定します。この比較は検査前後の状態を示すもので、途中で作成・削除されたファイルまで追跡するものではありません。

未対応の再解析ポイントは、[FSCTL_SET_REPARSE_POINT](https://learn.microsoft.com/en-us/windows/win32/api/winioctl/ni-winioctl-fsctl_set_reparse_point)と[REPARSE_GUID_DATA_BUFFER](https://learn.microsoft.com/en-us/windows/win32/api/winnt/ns-winnt-reparse_guid_data_buffer)の形式で、一時ディレクトリ内のファイル・空のディレクトリだけに模擬タグとGUIDを設定して再現します。ドライバーやサービスは導入しません。指定ファイル、共有リンクの参照先、途中のディレクトリ、スキルの親、直下のスキル候補、本文の参照、CLI候補の7経路を確認します。検査前に再解析データの読み戻しで設定を確かめ、検査後の状態比較を終えてから、テスト自身がタグを除去します。

## 通信と非実行の確認範囲

外部参照の試験では、ループバック上の接続検知器を用意し、そのアドレスを指すHTTP・HTTPSリンクを模擬本文へ入れます。検知器はテスト側が検査前後に接続して動作確認し、CLIによる追加のTCP接続がないことを確認します。模擬CLIも検査前にテスト側から起動して痕跡が残ることを確かめ、その痕跡を除去してから検査します。検知器と模擬CLIの動作確認は、検査対象に書かれた指示の実行ではなく、テスト自身が用意した操作です。

受入テストの「ネットワーク上の対象を判定不能にする」では、UNC・拡張長UNC・スラッシュ区切りの3表記を`--rules`・`--skills`へ渡し、`path_not_local`と終了コード2を確認します。本文の通常・参照形式・画像にはUNCや符号化したネットワークパスを入れ、記述位置付きの`reference_unsupported`と、参照検査だけが未完了になることを確認します。共有リンク経由では、指定ファイル、スキルの親、直下のスキル、`SKILL.md`、本文の参照、`PATH`、CLI候補の7経路を確認します。JSONと通常表示、分類別件数、検査前後の模擬データの一致も既存の共通処理で検査します。これらは拒否の結果を確認する試験で、接続の有無を監視するものではありません。

ネットワーク向けリンクの作成には、ファイルかディレクトリかを[CreateSymbolicLinkWのフラグ](https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-createsymboliclinkw)で明示し、作成側から参照先の属性を調べません。宛先はループバック上の模擬共有名だけで、実在する共有や認証情報を必要としません。

[TestReadOnlyDependencies](../cmd/rulecheck/safety_test.go)は`go list -deps`でテスト用コードを除く実行ファイルの依存を調べ、直接・間接に`net`または`os/exec`を含めば失敗します。構文解析用の`net/url`・`net/netip`は許容します。標準ライブラリの通信・プロセス起動機能が混入した場合の検出であり、Windows APIの直接呼出しやOSによる間接的な通信まで検出するものではありません。

主担当が別の観点から実装も確認しました。参照先とCLI候補はパス解決と種類の確認だけを使い、本文は明示した指示ファイルと直下のスキルでだけ読みます。リンク先もローカル専用の判定を通し、外部URIはファイルを開く前に対象外へ分けます。本体の依存パッケージ一覧に`net`・`net/http`・`os/exec`は含まれず、`net/url`・`net/netip`は含まれます。この確認は独立した評価ではありません。

接続検知器で観測するのは指定した接続先だけです。UNC・ネットワークリンク・割当ドライブの拒否確認と、この実測を合わせても、全通信がないと実測したことにはなりません。エージェントの実行環境は非管理者で、`pktmon status`もアクセス拒否となりました。[ETWセッションの制御に必要な権限](https://learn.microsoft.com/en-us/windows/win32/etw/controlling-event-tracing-sessions)を使う採取は利用者が実行し、後述の観測結果を得ました。同一試験での照合が未完了のため、M4の未確認事項として残します。模擬タグでの確認を、すべての製品固有の再解析ポイントの検証とは扱いません。

## 通信観測前の準備

通常のPowerShellで必須検証を終えてから、プロジェクトのルートで次を実行します。観測用の実行ファイル2つを、Git管理外の固有ディレクトリへ準備します。Goのビルド・依存取得はこの段階で終え、採取中には行いません。準備後にソースを変えた場合は、この準備と必須検証をやり直します。

```powershell
$rulecheckObserveDir = Join-Path (Get-Location) ('.cache/observation-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $rulecheckObserveDir | Out-Null
go build -trimpath -o (Join-Path $rulecheckObserveDir 'rulecheck.exe') ./cmd/rulecheck
if ($LASTEXITCODE -ne 0) { throw '観測用の実行ファイルを準備できませんでした。' }
go test -c -trimpath -o (Join-Path $rulecheckObserveDir 'rulecheck-tests.exe') ./cmd/rulecheck
if ($LASTEXITCODE -ne 0) { throw '観測用のテストを準備できませんでした。' }
Get-FileHash -LiteralPath (Join-Path $rulecheckObserveDir 'rulecheck.exe'), (Join-Path $rulecheckObserveDir 'rulecheck-tests.exe') |
    Select-Object Path, Hash | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $rulecheckObserveDir 'hashes.json') -Encoding utf8
$rulecheckObserveDir
```

`hashes.json`は観測対象とテスト本体の照合用です。再開する場合は、表示された絶対パスを`$rulecheckObserveDir`へ設定します。下記のテストはスキップも失敗として扱います。権限不足で共有リンク等を作れなければ、通常の利用者環境で必要な権限を整えて再実行してください。

## 通信観測の手順

今回の目的には、Windows標準の[pktmon](https://learn.microsoft.com/en-us/windows-server/networking/technologies/pktmon/pktmon)でパケットを採取し、[Process Monitor](https://learn.microsoft.com/en-us/sysinternals/downloads/procmon)で`rulecheck.exe`のファイル操作を確認する組合せを使います。どちらもこの環境に存在します。Wiresharkは採取した通信を詳しく解析するときに追加できます。pktmonはpcapngへ変換でき、[保存済みの採取ファイルの閲覧にはNpcapは不要](https://www.wireshark.org/docs/wsug_html_chunked/ChBuildInstallWinInstall.html)です。今回、追加の導入やドライバーの起動は行っていません。

1. 通常のPowerShellで上記の準備を終え、ビルド・依存取得と観測区間を分けます。割当ドライブがこの環境から見えることも、上記試験で先に確認します。
2. 別の管理者PowerShellで`pktmon status`と`pktmon filter list`を確認します。既存の採取やフィルターがあれば勝手に停止・削除せず、観測条件を調整します。全接続先を確認する試験では、DNSやSMBのポートだけに採取を限定しません。
3. Process Monitorは試験前から起動し、`Process Name is rulecheck.exe`と`Process Name is rulecheck-tests.exe`の包含フィルターを設定します。ファイル操作とネットワーク操作の両方を表示し、テスト本体の接続検知器によるTCP通信も記録して、ネットワーク操作の採取が有効だったことを確認します。既存の採取設定は別に保持し、ログの保存先はローカルのGit管理外とします。pktmonも既存セッションがなくフィルターもない状態で、下記の採取を開始します。64バイトの採取にも通信内容が含まれる場合があるため、生ログをコミットしません。

```powershell
$rulecheckTraceDir = Join-Path (Get-Location) ('.cache/network-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $rulecheckTraceDir | Out-Null
pktmon start --capture --pkt-size 64 --log-mode multi-file --file-size 64 --file-name (Join-Path $rulecheckTraceDir 'packets.etl')
if ($LASTEXITCODE -ne 0) { throw '通信の採取を開始できませんでした。' }
```

4. 観測器が通信を検出できることを、専用の検証先への既知の通信で確認します。検査の前後で行い、`rulecheck.exe`による通信と区別します。実在する個人用ファイルを読む操作は動作確認に使いません。
5. 元の通常のPowerShellで下記を実行します。Y:・Z:は、実際の割当ドライブ名に合わせます。準備済みのテストを直接起動し、各子プロセスのPID・UTCの開始終了時刻・終了コードをログに残します。引数や本文は観測区間のログに転載しません。テスト側の接続検知器と模擬CLIの動作確認を製品の動作へ数えず、記録した`rulecheck.exe`の区間を判定対象にします。

```powershell
$rulecheckPreviousBinary = $env:RULECHECK_TEST_BINARY
$rulecheckPreviousDrives = $env:RULECHECK_TEST_NETWORK_DRIVES
$rulecheckPreviousTrace = $env:RULECHECK_TEST_TRACE_PROCESSES
try {
    $env:RULECHECK_TEST_BINARY = Join-Path $rulecheckObserveDir 'rulecheck.exe'
    $env:RULECHECK_TEST_NETWORK_DRIVES = 'Y:;Z:'
    $env:RULECHECK_TEST_TRACE_PROCESSES = '1'
    & (Join-Path $rulecheckObserveDir 'rulecheck-tests.exe') '-test.v' '-test.run=^TestExecutable(Acceptance|MappedDrives)$' *> (Join-Path $rulecheckObserveDir 'acceptance.log')
    if ($LASTEXITCODE -ne 0) { throw '受入確認に失敗しました。採取も停止してください。' }
    if (Select-String -LiteralPath (Join-Path $rulecheckObserveDir 'acceptance.log') -Pattern '^\s*--- SKIP:') {
        throw '未確認の受入条件があります。ログを確認し、採取も停止してください。'
    }
}
finally {
    $env:RULECHECK_TEST_BINARY = $rulecheckPreviousBinary
    $env:RULECHECK_TEST_NETWORK_DRIVES = $rulecheckPreviousDrives
    $env:RULECHECK_TEST_TRACE_PROCESSES = $rulecheckPreviousTrace
}
```

6. 試験が失敗した場合も、採取を開始した管理者PowerShellで`pktmon stop`を実行します。実行ファイル2つのSHA-256が`hashes.json`と一致すること、受入ログの対象SHA-256が準備した`rulecheck.exe`と一致することを確認します。PIDと区間、採取の前後の動作確認、採取範囲、欠落の有無も記録します。解析用の変換は`pktmon etl2txt <ETLのパス>`または`pktmon etl2pcap <ETLのパス>`です。複数ファイルができた場合は全区間を確認します。
7. Process Monitorでは記録したPIDでファイル操作を絞り、Y:・Z:やUNCへの要求がないことを調べます。パケットをPIDだけで判断するとWindows経由の間接的な通信を見落とすため、同じ区間のDNS・SMB・他の通信も採取全体から照合します。常駐処理の通信などと区別できない場合は判定不能にします。

コマンドの引数は実機の`pktmon start help`・`pktmon etl2pcap help`と[公式の構文説明](https://learn.microsoft.com/en-us/windows-server/networking/technologies/pktmon/pktmon-syntax)で確認しています。パケットが見つからないだけでは合格にせず、前後の動作確認と採取範囲・欠落の評価を含めてM4を判定します。pktmonとProcess Monitorは同じ試験を採取し、テスト終了後に両方を停止してからCSVを保存します。再実行した試験に、前回の採取結果を組み合わせません。

## 利用者による観測と照合結果

2026年10月22日、利用者が管理者環境で通信を採取し、Process MonitorのCSVを提供しました。準備済み製品とテスト本体のSHA-256は、下記の記録と一致しています。以下の時刻は日本時間です。

| 証拠 | 試験との対応 | 確認結果 |
| --- | --- | --- |
| `capture-56357297cfb84e28b35f33a17d8d4229`の`packets1.etl`・`packets2.etl` | 採取区間20:21:12.1349975〜20:22:33.3799386。`run-b7b68370e67b48c5a32c16a7333fbc32`を含む | 利用者の停止結果はイベント欠落なし。後半のETLはイベント0件。前後のpingはそれぞれ要求7件・応答7件を採取地点別に記録。書式変換では先頭時刻に`0x490`のエラー2件があり、未解析事項として残す |
| 同試験の受入ログ | 製品71プロセスの区間は20:21:56.9719876〜20:21:57.7352071 | 基本9項目は成功、スキップ0件。割当ドライブの試験はY:の種別`1`で失敗。製品区間と重なるTCP・UDPの採取記録があるが、時刻だけでは製品への帰属を特定できない |
| 提供された`Logfile.CSV` | 20:26:07.7831630〜20:26:09.1034660。後の`run-af19a60cc15449e494db6b37bf04bcaf`の製品PID71個と完全一致 | 15,959件すべて製品の記録。各PIDの開始・終了も各1件。共有先へのアクセス、通常ファイルへの書込・削除・名前変更、TCP・UDP操作の記録は0件。`WriteFile`73件は名前付きパイプへの出力。本文の`ReadFile`64件は明示された指示・スキル内に限られ、参照先本文・CLI候補・模擬個人設定・入れ子のスキル本文の読取は0件 |

CSVのSHA-256は`ddd7af73d53ce94604cccbc7dd1c44b36ba0abf08ba6c50aaed805f12b642e44`です。CSVは後の試験に対応し、ETLの採取停止後でした。先の試験と共通するPID3個も、時刻が異なる再利用です。これらを同じ試験の証拠として合わせず、全通信の不存在は判定不能とします。CSVだけでは、ネットワーク操作の採取設定・除外・欠落も確定できません。生ログ、通信先、個人情報はGitへ含めません。

後の試験も基本9項目は成功し、割当ドライブの試験はY:の種別`1`で失敗しました。その後、利用者が通常のPowerShellで、管理者として実行していないことと、Y:・Z:の両方が`Network`であることを確認しました。失敗した試験とはドライブの見え方が異なります。手元の実行用スクリプトへ開始前のドライブ種別確認を追加し、割当が見えない場合は通信や受入試験の前に停止することを確認しました。

読み取り専用の補助担当がCSVのPID・時刻・操作内容を独立確認し、主担当もPID集合・時刻・件数を照合しました。この時点では同一試験の証拠がそろわず、同時採取による再確認を行いました。

## 同時採取による再確認

同日、`run-f4395577e80148ad9ed49f40d872864e`を`retry-20261022-2035`で採取しました。基本9項目とY:・Z:の割当ドライブ26項目はすべて成功し、スキップは0件でした。製品とテスト本体のSHA-256は下記の記録から変わっていません。

| 証拠 | 確認結果 |
| --- | --- |
| 受入ログと`process.CSV` | CSVは20:41:26.0101344〜20:41:28.4748592。全53,553件の内訳は製品26,590件・テスト本体26,963件。製品140起動をPIDと時刻で一意に照合でき、開始・終了も各140件ある。PIDは117種類で23個が再利用されているため、PIDだけでは照合しない |
| 製品の操作 | TCP・UDP操作、UNC・Y:・Z:・共有用デバイス経路へのアクセス、通常ファイルの作成・書込・削除・改名は記録0件。`WriteFile`142件は名前付きパイプへの出力。本文の`ReadFile`80件は明示された指示・直下スキル内で、参照先本文・CLI候補・模擬個人設定・入れ子のスキル本文の読取は0件 |
| Process Monitorの動作確認 | テスト本体のTCP14件を成功として記録。接続・受付・複製・送信・受信が各2件、切断が4件。製品と同じ採取区間でネットワーク操作の採取が有効だったことを確認した |
| `packets1.etl`・`packets2.etl` | 採取区間20:41:09.7039920〜20:41:39.8184609で試験の前後を含む。前後のpingはそれぞれ要求7件・応答7件を採取地点別に記録。後半のETLはイベント0件。`tracerpt`で両ファイルを処理した結果は92,095イベント・欠落0件 |

CSVのSHA-256は`88b0458ebeaf084de695488e78a2ce16a1eae979c9873e48f60142a90b82ab34`です。原本・集計とハッシュの対応は、Git管理外の同じ採取ディレクトリへ保存しました。読み取り専用の補助担当がCSVを独立確認し、主担当も件数・PID・時刻・終了コードを照合しました。

`pktmon etl2txt`には採取先頭の`0x490`が2件あります。別の集計で、パケットイベント160・170の合計91,565件と変換後のパケット記録が一致し、製品区間内に書式変換エラーはありませんでした。欠落0件は[tracerpt](https://learn.microsoft.com/en-us/windows-server/administration/windows-commands/tracerpt)の集計によるもので、書式変換の成功とは区別しています。

製品区間には背景のTCP・UDP等の通信も重なっています。製品自身のネットワーク操作が0件であることは確認できましたが、間接的な通信との切り分けには、同じProcess Monitorで採取された他プロセスの記録が必要です。追加保存を依頼したところ、利用者から元の採取データは残っていないと回答がありました。現在の証拠だけでは背景通信の帰属を確定できないため、M4は進行中とします。再観測する際は、全イベントをファイルへ保存する設定を採取前に確定し、停止後にも原本を残します。

## 確認結果

Windows/amd64・Go 1.27.3で、固定済みモジュールのキャッシュを使い、`GOPROXY=off`・`GOTOOLCHAIN=local`を指定して確認しました。`GOCACHE`はGit管理外の`.cache/go-build`、`GOMODCACHE`は既存の`.cache/go-mod`を使いました。新たな取得や依存の版変更は行っていません。

必須検証が成功し、`bin/rulecheck.exe --version`と正本の一致を確認しました。さらに、準備済みのテスト本体を直接実行し、`PATH`を観測用ディレクトリだけに限定してGoを検索できない状態でも、既存の受入テスト9項目と、その中のネットワーク対象11項目が成功しました。共有リンクと再解析ポイントを含めスキップは0件、PID・UTCの時刻・終了コードの記録も確認しました。相対パス、不存在の実行ファイル、版番号が一致しない実行ファイルの3条件では、それぞれ想定した理由で失敗し、代替ビルドがないことを確認しました。

割当ドライブの試験は、最初の試行ではY:の種別`1`で失敗しました。その後、割当が見える通常のPowerShellから実行し、今回の対象でもY:・Z:の26項目が成功、スキップ0件となりました。通常の必須検証では、この追加試験だけが未指定のためスキップされます。利用者による採取とCSVの照合結果は上記へ記録しました。

今回準備した実行ファイルと必須検証の実行ファイルのSHA-256は、ともに`bdeb78e65780a462399532fb898436877a1d676330812ab36c65b4bf414e4f91`です。テスト本体のSHA-256は`ed2cf3de8313ae3ec14ec6488bff972187d23fc4bd56ee1d10429d35badfe375`です。手元の準備済み成果物とログは`.cache/observation-20261022-37347/`へ置き、Gitの対象から除外しています。これは上記基準コミットに今回の変更を加えた作業状態の証拠であり、後のコミットや再ビルドで同じハッシュになることを保証しません。

前回の必須検証は、従来の`gofmt -l .`がGit管理外の依存キャッシュも検査して停止しました。依存ソースを変更せず、ソースを置く`cmd/`と`internal/`の配下を`gofmt`で再帰的に検査するよう修正しました。通常のビルドでは除外される独自タグ付きファイルと、そのファイルだけで構成したパッケージでも書式違反を検出することを確認し、検証用ファイルは除去しました。

前回は読み取り専用の補助担当が、検査実装・再解析データの作成と後処理・前後比較を静的に独立確認しました。書式検査の変更案には、ビルド対象外のファイルだけのパッケージを見落とす指摘があり、上記の再帰的な検査へ修正しました。テストの実行は主担当によるものです。

前回は読み取り専用の補助担当が、UNC等のネットワーク対象の追加試験と依存検査、関連する検査本体の経路を静的に独立確認し、指摘はありませんでした。今回は別の読み取り専用の補助担当が、準備済み実行ファイルを指定する分岐、既存の受入条件、観測時のGoコマンドの非起動と手順を静的に独立確認し、指摘はありませんでした。実行検証は主担当が行いました。通信の不存在を実測していないという限界は変わりません。

前回は検査本体の不具合修正としてCLI版を`0.1.0`から`0.1.1`へ更新しました。今回はテストと開発文書だけの変更のため、READMEの版管理規則に従い`0.1.1`を据え置きます。JSON形式も`1`を維持しました。
