[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [string]$ObservationDirectory,
    [string]$NetworkDrives = '',
    [switch]$CheckOnly
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false

$artifactDirectory = (Resolve-Path -LiteralPath $ObservationDirectory).ProviderPath
$hashPath = Join-Path $artifactDirectory 'hashes.json'
$artifactNames = @('rulecheck.exe', 'rulecheck-tests.exe')
$expectedHashes = @{}
try {
    $records = @(Get-Content -LiteralPath $hashPath -Raw | ConvertFrom-Json)
    if ($records.Count -ne $artifactNames.Count) {
        throw '実行ファイルは2件必要です。'
    }
    foreach ($record in $records) {
        $name = [IO.Path]::GetFileName($record.Path)
        if ($name -notin $artifactNames -or $expectedHashes.ContainsKey($name) -or
            $record.Hash -notmatch '^[0-9a-fA-F]{64}$' -or
            $record.Path -ne (Join-Path $artifactDirectory $name)) {
            throw '実行ファイルのパス・件数・SHA-256が不正です。'
        }
        $expectedHashes[$name] = $record.Hash
    }
}
catch {
    throw 'hashes.jsonを読み取れないか形式が不正です。受入確認の「通信観測前の準備」をやり直してください。'
}

function Confirm-ObservationArtifacts {
    foreach ($name in $artifactNames) {
        $path = Join-Path $artifactDirectory $name
        if ((Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash -ne $expectedHashes[$name]) {
            throw "準備済み成果物が変更されています。採取中なら停止してください: $name"
        }
    }
}
Confirm-ObservationArtifacts

# ルートの種別だけを確認し、割当先の存在確認や列挙はしない。
$drives = @()
if ($NetworkDrives -ne '') {
    foreach ($entry in $NetworkDrives.Split(';')) {
        $drive = $entry.Trim().ToUpperInvariant()
        if ($drive -notmatch '^[A-Z]:$') {
            throw '割当ドライブはY:;Z:のように指定してください。'
        }
        $kind = [IO.DriveInfo]::new($drive + '\').DriveType
        Write-Output "$drive の種別: $kind"
        if ($kind -ne [IO.DriveType]::Network) {
            throw "$drive の割当がこのPowerShellから見えません。割当が見える通常のPowerShellで実行してください。採取中なら停止してください。"
        }
        if ($drive -notin $drives) {
            $drives += $drive
        }
    }
}

if ($CheckOnly) {
    Write-Output '成果物のSHA-256と指定された割当ドライブを確認しました。受入試験は実行していません。'
    return
}

$testPattern = '^TestExecutableAcceptance$'
if ($drives.Count -ne 0) {
    $testPattern = '^TestExecutable(Acceptance|MappedDrives)$'
}
else {
    Write-Output '割当ドライブ未指定: 基本の受入試験だけを実行します。割当ドライブの実機確認は未確認として残ります。'
}

# 再試験では別の保存先を作り、前回の証拠を上書きしない。
$runDirectory = Join-Path $artifactDirectory ('run-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $runDirectory | Out-Null
$logPath = Join-Path $runDirectory 'acceptance.log'
Write-Output "ログ保存先: $runDirectory"

$previousBinary = $env:RULECHECK_TEST_BINARY
$previousDrives = $env:RULECHECK_TEST_NETWORK_DRIVES
$previousTrace = $env:RULECHECK_TEST_TRACE_PROCESSES
$testExit = -1
$started = [DateTime]::UtcNow.ToString('o')
try {
    $env:RULECHECK_TEST_BINARY = Join-Path $artifactDirectory 'rulecheck.exe'
    $env:RULECHECK_TEST_NETWORK_DRIVES = $drives -join ';'
    $env:RULECHECK_TEST_TRACE_PROCESSES = '1'
    & (Join-Path $artifactDirectory 'rulecheck-tests.exe') '-test.v' '-test.count=1' "-test.run=$testPattern" *> $logPath
    $testExit = $LASTEXITCODE
}
finally {
    $ended = [DateTime]::UtcNow.ToString('o')
    $env:RULECHECK_TEST_BINARY = $previousBinary
    $env:RULECHECK_TEST_NETWORK_DRIVES = $previousDrives
    $env:RULECHECK_TEST_TRACE_PROCESSES = $previousTrace
    [ordered]@{
        started_utc = $started
        ended_utc = $ended
        test_exit_code = $testExit
        network_drives = $drives
        artifacts = $expectedHashes
    } | ConvertTo-Json -Depth 3 | Set-Content -LiteralPath (Join-Path $runDirectory 'run.json') -Encoding utf8
    Confirm-ObservationArtifacts
}

Select-String -LiteralPath $logPath -Pattern '検証対象:|^--- |^\s+--- SKIP:|^PASS$|^FAIL$' |
    ForEach-Object { $_.Line }
if ($testExit -ne 0) {
    throw "受入確認に失敗しました。採取も停止し、ログを確認してください: $logPath"
}
if (Select-String -LiteralPath $logPath -Pattern '^\s*--- SKIP:') {
    throw "未確認の受入条件があります。採取も停止し、ログを確認してください: $logPath"
}
$requiredTests = @('TestExecutableAcceptance')
if ($drives.Count -ne 0) {
    $requiredTests += 'TestExecutableMappedDrives'
}
foreach ($name in $requiredTests) {
    if (-not (Select-String -LiteralPath $logPath -Pattern "^--- PASS: $name \(")) {
        throw "必要な受入試験の成功記録がありません。採取も停止し、ログを確認してください: $logPath"
    }
}
Write-Output '選択した受入試験が成功しました。通信観測の判定は、同じ試験で採取した原本との照合後に行います。'
