[CmdletBinding()]
param(
    [Parameter(Mandatory)][ValidatePattern('^[0-9a-f]{40}$')][string]$Before,
    [Parameter(Mandatory)][ValidatePattern('^[0-9a-f]{40}$')][string]$Commit
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Get-VersionAtCommit([string]$Revision) {
    $source = & git show "${Revision}:internal/version/version.go"
    if ($LASTEXITCODE -ne 0) {
        throw 'push前後の版番号を取得できません。コミット履歴を確認してください。'
    }
    $pattern = '(?m)^const Current = "((?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*))"\r?$'
    $versions = [regex]::Matches(($source -join "`n"), $pattern)
    if ($versions.Count -ne 1) {
        throw '版番号の正本を読み取れません。'
    }
    return $versions[0].Groups[1].Value
}

$version = Get-VersionAtCommit $Commit
$previous = if ($Before -eq ('0' * 40)) { $null } else { Get-VersionAtCommit $Before }
if ($previous -and [version]$version -lt [version]$previous) {
    throw '版番号が下がっています。公開済みの版へ戻さず、新しい番号を使ってください。'
}

[pscustomobject]@{
    Version = $version
    Changed = $version -cne $previous
}
