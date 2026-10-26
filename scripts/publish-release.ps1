[CmdletBinding()]
param(
    [Parameter(Mandatory)][ValidatePattern('^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$')][string]$Version,
    [Parameter(Mandatory)][ValidatePattern('^[0-9a-f]{40}$')][string]$Commit,
    [Parameter(Mandatory)][ValidatePattern('^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$')][string]$Repository
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Invoke-GitHub([string[]]$Arguments) {
    $result = & gh @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw 'GitHubでの公開処理に失敗しました。状態を確認してから同じ実行を再実行してください。'
    }
    return $result
}

$projectRoot = Split-Path -Parent $PSScriptRoot
$releaseDirectory = Join-Path $projectRoot "bin/release/$Version"
$tag = "v$Version"
$assetNames = @("rulecheck_${Version}_windows_amd64.zip", 'checksums.txt', 'rulecheck.json')
foreach ($name in $assetNames) {
    if (-not (Test-Path -LiteralPath (Join-Path $releaseDirectory $name) -PathType Leaf)) {
        throw "配布物がありません: $name"
    }
}

# 一覧取得の失敗とReleaseが存在しない状態を区別し、認証・通信エラーでは作成へ進まない。
$query = ".[] | select(.tag_name == `"$tag`") | {id, draft, target_commitish, assets: [.assets[].name]}"
$existing = @(Invoke-GitHub @('api', '--paginate', "repos/$Repository/releases?per_page=100", '--jq', $query) | ConvertFrom-Json)
if ($existing.Count -gt 1) {
    throw '同じタグのReleaseが複数あります。公開状態を確認してください。'
}

function Test-ReleaseTag {
    # ローカル取得後に作成されたタグも確認し、注釈付きタグはコミットへ解決する。
    $refQuery = ".[] | select(.ref == `"refs/tags/$tag`") | .ref"
    $reference = Invoke-GitHub @('api', "repos/$Repository/git/matching-refs/tags/$tag", '--jq', $refQuery)
    if (-not $reference) { return $false }
    $tagCommit = Invoke-GitHub @('api', "repos/$Repository/commits/$tag", '--jq', '.sha')
    if ($tagCommit -cne $Commit) {
        throw '同じ版のタグが別コミットを指しています。新しい番号を使ってください。'
    }
    return $true
}

$tagExists = Test-ReleaseTag
if ($existing.Count -eq 1 -and -not $existing[0].draft) {
    if (-not $tagExists) { throw '公開済みReleaseのタグを確認できません。' }
    Write-Output "$tag は公開済みです。添付ファイルは変更しません。"
    return
}

$uploadedNames = @()
if ($existing.Count -eq 1) {
    if ($existing[0].target_commitish -cne $Commit) {
        throw '下書きReleaseの対象コミットが一致しません。公開状態を確認してください。'
    }
    $uploadedNames = @($existing[0].assets)
    if (@($uploadedNames | Where-Object { $_ -cnotin $assetNames }).Count -ne 0) {
        throw '下書きReleaseに想定外の添付ファイルがあります。公開状態を確認してください。'
    }
}
if (-not $tagExists) {
    # 作成競合はAPIの失敗として止める。既存タグの上書きや移動は行わない。
    Invoke-GitHub @('api', '--method', 'POST', "repos/$Repository/git/refs",
        '--raw-field', "ref=refs/tags/$tag", '--raw-field', "sha=$Commit") | Out-Null
}
if ($existing.Count -eq 0) {
    Invoke-GitHub @('release', 'create', $tag, '--repo', $Repository, '--target', $Commit,
        '--verify-tag', '--title', "rulecheck $tag", '--generate-notes', '--draft') | Out-Null
}

$missing = @($assetNames | Where-Object { $_ -cnotin $uploadedNames } | ForEach-Object { Join-Path $releaseDirectory $_ })
if ($missing.Count -ne 0) {
    # 既存の添付ファイルは上書きせず、途中失敗からの再実行では不足分だけを追加する。
    Invoke-GitHub (@('release', 'upload', $tag, '--repo', $Repository) + $missing) | Out-Null
}
$downloadDirectory = Join-Path $releaseDirectory ("download-" + [guid]::NewGuid().ToString('N'))
[System.IO.Directory]::CreateDirectory($downloadDirectory) | Out-Null
Invoke-GitHub @('release', 'download', $tag, '--repo', $Repository, '--dir', $downloadDirectory) | Out-Null
foreach ($name in $assetNames) {
    $localHash = (Get-FileHash -LiteralPath (Join-Path $releaseDirectory $name) -Algorithm SHA256).Hash
    $remoteHash = (Get-FileHash -LiteralPath (Join-Path $downloadDirectory $name) -Algorithm SHA256).Hash
    if ($localHash -cne $remoteHash) {
        throw "添付ファイルが生成物と一致しません。下書きを確認してください: $name"
    }
}
if (-not (Test-ReleaseTag)) { throw '公開直前にタグを確認できません。' }
$ready = @(Invoke-GitHub @('api', '--paginate', "repos/$Repository/releases?per_page=100", '--jq', $query) | ConvertFrom-Json)
if ($ready.Count -ne 1 -or -not $ready[0].draft -or $ready[0].target_commitish -cne $Commit -or
    $ready[0].assets.Count -ne $assetNames.Count -or @($ready[0].assets | Where-Object { $_ -cnotin $assetNames }).Count -ne 0) {
    throw '公開直前の下書き・対象コミット・添付ファイルを確認できません。'
}
# 完了順が逆転しても、版番号に基づいてLatestを選ぶ。
Invoke-GitHub @('api', '--method', 'PATCH', "repos/$Repository/releases/$($ready[0].id)",
    '--field', 'draft=false', '--raw-field', 'make_latest=legacy') | Out-Null
Write-Output "公開しました: https://github.com/$Repository/releases/tag/$tag"
