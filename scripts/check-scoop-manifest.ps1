[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$versionSource = Get-Content -LiteralPath (Join-Path $projectRoot 'internal/version/version.go') -Raw
if ($versionSource -notmatch '(?m)^const Current = "((?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*))"\r?$') {
    throw '版番号の正本を読み取れません。'
}
$version = $Matches[1]
try {
    $manifest = Get-Content -LiteralPath (Join-Path $projectRoot 'bucket/rulecheck.json') -Raw | ConvertFrom-Json -AsHashtable
}
catch { throw 'Scoopの配布定義を読み取れません。配布物を生成してbucket/rulecheck.jsonへ反映してください。' }
if ($manifest -isnot [System.Collections.IDictionary] -or $manifest['version'] -isnot [string] -or $manifest['version'] -cne $version) {
    throw 'Scoopの版番号が正本と一致しません。配布物を生成してbucket/rulecheck.jsonへ反映してください。'
}
$architecture = $manifest['architecture']
if ($architecture -isnot [System.Collections.IDictionary] -or $architecture['64bit'] -isnot [System.Collections.IDictionary]) {
    throw 'ScoopのWindows x64用の配布定義がありません。'
}
$platform = $architecture['64bit']
$expectedUrl = "https://github.com/shouhon/rulecheck/releases/download/v$version/rulecheck_${version}_windows_amd64.zip"
if ($platform['url'] -isnot [string] -or $platform['url'] -cne $expectedUrl -or
    $platform['hash'] -isnot [string] -or $platform['hash'] -cnotmatch '^[0-9a-f]{64}$' -or
    $manifest['bin'] -isnot [string] -or $manifest['bin'] -cne 'rulecheck.exe') {
    throw 'Scoopの配布先・SHA-256・実行ファイルの定義が正しくありません。生成した定義を反映してください。'
}
Write-Output 'Scoopの配布定義と版番号の一致を確認しました。'
