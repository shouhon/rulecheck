[CmdletBinding()]
param(
    [string]$OutputDirectory = 'bin/release'
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

if (-not $IsWindows -or [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture -ne 'X64') {
    throw '配布物の生成と起動確認はWindows x64で実行してください。'
}

$projectRoot = Split-Path -Parent $PSScriptRoot
Push-Location -LiteralPath $projectRoot
$savedEnvironment = @{}
foreach ($name in @('GOOS', 'GOARCH', 'CGO_ENABLED')) {
    $savedEnvironment[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
}
try {
    $versionSource = Get-Content -LiteralPath 'internal/version/version.go' -Raw
    if ($versionSource -notmatch 'const Current = "(\d+\.\d+\.\d+)"') {
        throw '版番号の正本を読み取れません。'
    }
    $version = $Matches[1]
    $outputRoot = [System.IO.Path]::GetFullPath($OutputDirectory, $projectRoot)
    $releaseDirectory = Join-Path $outputRoot $version
    [System.IO.Directory]::CreateDirectory($releaseDirectory) | Out-Null

    $env:GOOS = 'windows'
    $env:GOARCH = 'amd64'
    $env:CGO_ENABLED = '0'
    $executable = Join-Path $releaseDirectory 'rulecheck.exe'
    # コミット前後の作業ツリー状態を実行ファイルへ埋め込まず、ソースはReleaseの対象コミットで特定する。
    & go build -trimpath -buildvcs=false -o $executable ./cmd/rulecheck
    if ($LASTEXITCODE -ne 0) {
        throw '配布用実行ファイルのビルドに失敗しました。'
    }
    $reportedVersion = & $executable --version
    if ($LASTEXITCODE -ne 0 -or $reportedVersion -cne "rulecheck $version") {
        throw '実行ファイルの版番号が正本と一致しません。'
    }

    $archiveName = "rulecheck_${version}_windows_amd64.zip"
    $archivePath = Join-Path $releaseDirectory $archiveName
    $archive = [System.IO.Compression.ZipFile]::Open($archivePath, 'Create')
    try {
        $files = [ordered]@{
            'rulecheck.exe' = $executable
            'LICENSE' = Join-Path $projectRoot 'LICENSE'
            'THIRD_PARTY_NOTICES.txt' = Join-Path $projectRoot 'THIRD_PARTY_NOTICES.txt'
        }
        foreach ($name in $files.Keys) {
            $entry = $archive.CreateEntry($name, [System.IO.Compression.CompressionLevel]::Optimal)
            # 入力の更新日時に依存しないZIPにする。
            $entry.LastWriteTime = [DateTimeOffset]::new(1980, 1, 1, 0, 0, 0, [TimeSpan]::Zero)
            $source = [System.IO.File]::OpenRead($files[$name])
            try {
                $destination = $entry.Open()
                try { $source.CopyTo($destination) }
                finally { $destination.Dispose() }
            }
            finally { $source.Dispose() }
        }
    }
    finally { $archive.Dispose() }

    $hash = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
    $manifest = [ordered]@{
        version = $version
        description = '指示ファイルとスキルの参照先・共有リンク・必要なCLIを読み取り専用で検査'
        homepage = 'https://github.com/shouhon/rulecheck'
        license = 'MIT'
        architecture = [ordered]@{
            '64bit' = [ordered]@{
                url = "https://github.com/shouhon/rulecheck/releases/download/v$version/$archiveName"
                hash = $hash
            }
        }
        bin = 'rulecheck.exe'
    }
    $manifestPath = Join-Path $releaseDirectory 'rulecheck.json'
    $manifestJson = ($manifest | ConvertTo-Json -Depth 5).Replace("`r`n", "`n")
    [System.IO.File]::WriteAllText($manifestPath, $manifestJson + "`n")
    $manifestHash = (Get-FileHash -LiteralPath $manifestPath -Algorithm SHA256).Hash.ToLowerInvariant()
    $checksums = "$hash  $archiveName`n$manifestHash  rulecheck.json`n"
    [System.IO.File]::WriteAllText((Join-Path $releaseDirectory 'checksums.txt'), $checksums)
    Write-Output "配布物を生成しました: $releaseDirectory"
    Write-Output "ZIPのSHA-256: $hash"
}
finally {
    foreach ($name in $savedEnvironment.Keys) {
        [Environment]::SetEnvironmentVariable($name, $savedEnvironment[$name], 'Process')
    }
    Pop-Location
}
