[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$temporaryRoot = [System.IO.Path]::GetFullPath([System.IO.Path]::GetTempPath())
$sandbox = Join-Path $temporaryRoot ('rulecheck-release-test-' + [guid]::NewGuid().ToString('N'))
[System.IO.Directory]::CreateDirectory($sandbox) | Out-Null
$global:RulecheckReleaseTest_assertions = 0

function Assert-True([bool]$Condition, [string]$Message) {
    if (-not $Condition) { throw $Message }
    $global:RulecheckReleaseTest_assertions++
}

function Assert-Throws([scriptblock]$Action, [string]$Message) {
    $failed = $false
    try { & $Action | Out-Null }
    catch { $failed = $true }
    Assert-True $failed $Message
}

function Invoke-TestGit([string[]]$Arguments) {
    $result = & git @Arguments
    if ($LASTEXITCODE -ne 0) { throw '模擬リポジトリの操作に失敗しました。' }
    return $result
}

function New-TestCommit([string]$Source) {
    [System.IO.File]::WriteAllText((Join-Path $sandbox 'internal/version/version.go'), $Source)
    Invoke-TestGit @('add', '--', '.') | Out-Null
    Invoke-TestGit @('-c', 'user.name=リリース検証', '-c', 'user.email=release-test@example.invalid',
        'commit', '--quiet', '-m', 'test: 模擬データを更新') | Out-Null
    return Invoke-TestGit @('rev-parse', 'HEAD')
}

try {
    [System.IO.Directory]::CreateDirectory((Join-Path $sandbox 'internal/version')) | Out-Null
    Push-Location -LiteralPath $sandbox
    try {
        Invoke-TestGit @('init', '--quiet') | Out-Null
        $before = New-TestCommit "package version`nconst Current = `"0.1.2`"`n"
        [System.IO.File]::WriteAllText((Join-Path $sandbox 'memo.md'), '版番号を変えない変更')
        $unchanged = New-TestCommit "package version`nconst Current = `"0.1.2`"`n"
        $selector = Join-Path $projectRoot 'scripts/get-release-info.ps1'
        $result = & $selector -Before $before -Commit $unchanged
        Assert-True (-not $result.Changed) '版番号の据え置きで公開対象になりました。'

        $bumped = New-TestCommit "package version`nconst Current = `"0.1.3`"`n"
        [System.IO.File]::WriteAllText((Join-Path $sandbox 'memo.md'), '版番号更新後の別コミット')
        $after = New-TestCommit "package version`nconst Current = `"0.1.3`"`n"
        $result = & $selector -Before $unchanged -Commit $after
        Assert-True ($result.Changed -and $result.Version -ceq '0.1.3') '複数コミットのpushで版番号変更を検出できません。'
        $result = & $selector -Before ('0' * 40) -Commit $after
        Assert-True $result.Changed '最初のpushを判定できません。'

        $lower = New-TestCommit "package version`nconst Current = `"0.1.1`"`n"
        Assert-Throws { & $selector -Before $bumped -Commit $lower } '版番号の引き下げを許可しました。'
        $invalid = New-TestCommit "package version`nconst Current = `"0.01.4`"`n"
        Assert-Throws { & $selector -Before $bumped -Commit $invalid } '不正な版番号を許可しました。'
        Assert-Throws { & $selector -Before ('f' * 40) -Commit $after 2>$null } '取得できない比較元を無視しました。'
    }
    finally { Pop-Location }

    # 配布定義と公開処理は隔離した模擬データで確認し、GitHubへ接続しない。
    [System.IO.Directory]::CreateDirectory((Join-Path $sandbox 'scripts')) | Out-Null
    [System.IO.Directory]::CreateDirectory((Join-Path $sandbox 'bucket')) | Out-Null
    $manifestChecker = Join-Path $sandbox 'scripts/check-scoop-manifest.ps1'
    Copy-Item -LiteralPath (Join-Path $projectRoot 'scripts/check-scoop-manifest.ps1') -Destination $manifestChecker
    [System.IO.File]::WriteAllText((Join-Path $sandbox 'internal/version/version.go'), "package version`nconst Current = `"9.8.7`"`n")
    $manifestPath = Join-Path $sandbox 'bucket/rulecheck.json'
    $validManifest = @{
        version = '9.8.7'
        architecture = @{ '64bit' = @{
            url = 'https://github.com/shouhon/rulecheck/releases/download/v9.8.7/rulecheck_9.8.7_windows_amd64.zip'
            hash = 'a' * 64
        } }
        bin = 'rulecheck.exe'
    } | ConvertTo-Json -Depth 5
    [System.IO.File]::WriteAllText($manifestPath, $validManifest)
    & $manifestChecker | Out-Null
    foreach ($change in @(
        { param($m) $m.version = '9.8.6' },
        { param($m) $m.version = @('9.8.7') },
        { param($m) $m.architecture['64bit'].url = 'https://example.invalid/other.zip' },
        { param($m) $m.architecture['64bit'].hash = '不正なハッシュ' },
        { param($m) $m.bin = 'other.exe' },
        { param($m) $m.architecture = @{} }
    )) {
        $manifest = $validManifest | ConvertFrom-Json -AsHashtable
        & $change $manifest
        [System.IO.File]::WriteAllText($manifestPath, ($manifest | ConvertTo-Json -Depth 5))
        Assert-Throws { & $manifestChecker } '不整合なScoopの配布定義を許可しました。'
    }
    [System.IO.File]::WriteAllText($manifestPath, '{')
    Assert-Throws { & $manifestChecker } '不正なJSONのScoop配布定義を許可しました。'

    $publisher = Join-Path $sandbox 'scripts/publish-release.ps1'
    Copy-Item -LiteralPath (Join-Path $projectRoot 'scripts/publish-release.ps1') -Destination $publisher
    $releaseDirectory = Join-Path $sandbox 'bin/release/9.8.7'
    [System.IO.Directory]::CreateDirectory($releaseDirectory) | Out-Null
    foreach ($name in @('rulecheck_9.8.7_windows_amd64.zip', 'checksums.txt', 'rulecheck.json')) {
        [System.IO.File]::WriteAllText((Join-Path $releaseDirectory $name), "模擬配布物: $name")
    }
    $commit = 'a' * 40

    function Reset-GitHub {
        $global:RulecheckReleaseTest_exists = $false
        $global:RulecheckReleaseTest_draft = $true
        $global:RulecheckReleaseTest_target = $commit
        $global:RulecheckReleaseTest_tagCommit = $null
        $global:RulecheckReleaseTest_remoteAssets = @{}
        $global:RulecheckReleaseTest_calls = [System.Collections.Generic.List[string]]::new()
        $global:RulecheckReleaseTest_failAt = ''
        $global:RulecheckReleaseTest_corruptDownload = $false
        $global:RulecheckReleaseTest_replaceTagAfterDownload = $false
    }

    function gh {
        $call = @($args)
        $operation = if ($call[0] -ceq 'api' -and $call[1] -ceq '--method') { 'api ' + $call[2] } else { $call[0] + ' ' + $call[1] }
        $global:RulecheckReleaseTest_calls.Add($operation)
        $global:LASTEXITCODE = 0
        if ($global:RulecheckReleaseTest_failAt -ceq $operation) { $global:LASTEXITCODE = 1; return }
        switch ($operation) {
            'api --paginate' {
                if ($global:RulecheckReleaseTest_exists) {
                    return (@{ id = 1; draft = $global:RulecheckReleaseTest_draft; target_commitish = $global:RulecheckReleaseTest_target;
                        assets = @($global:RulecheckReleaseTest_remoteAssets.Keys) } | ConvertTo-Json -Compress)
                }
            }
            'api POST' {
                Assert-True (-not $global:RulecheckReleaseTest_tagCommit) '既存タグを変更しました。'
                Assert-True (("sha=$commit") -cin $call) '検証したコミット以外へタグを作成しました。'
                $global:RulecheckReleaseTest_tagCommit = $commit
            }
            'release create' {
                Assert-True ('--verify-tag' -cin $call -and $global:RulecheckReleaseTest_tagCommit -ceq $commit) 'タグの確認前に下書きを作成しました。'
                $global:RulecheckReleaseTest_exists = $true
            }
            'release upload' {
                Assert-True ('--clobber' -cnotin $call) '既存添付ファイルの上書きを要求しました。'
                foreach ($path in $call[5..($call.Count - 1)]) {
                    $name = [System.IO.Path]::GetFileName($path)
                    Assert-True (-not $global:RulecheckReleaseTest_remoteAssets.ContainsKey($name)) '同じ添付ファイルを再送信しました。'
                    $global:RulecheckReleaseTest_remoteAssets[$name] = [System.IO.File]::ReadAllBytes($path)
                }
            }
            'release download' {
                $directory = $call[[array]::IndexOf($call, '--dir') + 1]
                foreach ($name in $global:RulecheckReleaseTest_remoteAssets.Keys) {
                    [System.IO.File]::WriteAllBytes((Join-Path $directory $name), $global:RulecheckReleaseTest_remoteAssets[$name])
                }
                if ($global:RulecheckReleaseTest_corruptDownload) {
                    [System.IO.File]::WriteAllText((Join-Path $directory 'checksums.txt'), '破損')
                }
                if ($global:RulecheckReleaseTest_replaceTagAfterDownload) {
                    $global:RulecheckReleaseTest_tagCommit = 'b' * 40
                }
            }
            'api PATCH' {
                Assert-True ('draft=false' -cin $call -and 'make_latest=legacy' -cin $call) '公開順に依存しないLatestの選択を指定していません。'
                $global:RulecheckReleaseTest_draft = $false
            }
            default {
                if ($call[0] -ceq 'api' -and $call[1] -like '*/git/matching-refs/tags/v9.8.7') {
                    if ($global:RulecheckReleaseTest_tagCommit) { return 'refs/tags/v9.8.7' }
                    return
                }
                if ($call[0] -ceq 'api' -and $call[1] -like '*/commits/v9.8.7') {
                    return $global:RulecheckReleaseTest_tagCommit
                }
                throw "想定外のGitHub操作です: $operation"
            }
        }
    }

    function Publish-TestRelease {
        & $publisher -Version '9.8.7' -Commit $commit -Repository 'example/rulecheck' | Out-Null
    }

    Reset-GitHub
    Publish-TestRelease
    Assert-True (-not $global:RulecheckReleaseTest_draft -and $global:RulecheckReleaseTest_remoteAssets.Count -eq 3) '配布物3件の検証と公開が完了しません。'
    $global:RulecheckReleaseTest_calls.Clear()
    Publish-TestRelease
    Assert-True (@($global:RulecheckReleaseTest_calls | Where-Object { $_ -like 'release *' -or $_ -cin @('api POST', 'api PATCH') }).Count -eq 0) '公開済みの同じ版を変更しました。'

    Reset-GitHub
    $global:RulecheckReleaseTest_failAt = 'release upload'
    Assert-Throws { Publish-TestRelease } '送信失敗を無視しました。'
    Assert-True $global:RulecheckReleaseTest_draft '送信失敗後に公開しました。'
    # 部分的な送信成功後も、不足分だけを送って再開できることを確認する。
    $global:RulecheckReleaseTest_remoteAssets['checksums.txt'] = [System.IO.File]::ReadAllBytes((Join-Path $releaseDirectory 'checksums.txt'))
    $global:RulecheckReleaseTest_failAt = ''
    Publish-TestRelease
    Assert-True (-not $global:RulecheckReleaseTest_draft -and @($global:RulecheckReleaseTest_calls | Where-Object { $_ -ceq 'release create' }).Count -eq 1) '下書きから再開できません。'

    Reset-GitHub
    $global:RulecheckReleaseTest_corruptDownload = $true
    Assert-Throws { Publish-TestRelease } '添付ファイルの不一致を無視しました。'
    Assert-True ($global:RulecheckReleaseTest_draft -and 'api PATCH' -cnotin $global:RulecheckReleaseTest_calls) '照合失敗後に公開しました。'

    Reset-GitHub
    $global:RulecheckReleaseTest_replaceTagAfterDownload = $true
    Assert-Throws { Publish-TestRelease } '公開直前のタグの不一致を無視しました。'
    Assert-True ($global:RulecheckReleaseTest_draft -and 'api PATCH' -cnotin $global:RulecheckReleaseTest_calls) 'タグの不一致後に公開しました。'

    Reset-GitHub
    $global:RulecheckReleaseTest_failAt = 'api POST'
    Assert-Throws { Publish-TestRelease } 'タグ作成の競合を無視しました。'
    Assert-True ('release create' -cnotin $global:RulecheckReleaseTest_calls) 'タグ作成の失敗後に下書きを作成しました。'

    Reset-GitHub
    $global:RulecheckReleaseTest_exists = $true
    $global:RulecheckReleaseTest_target = 'b' * 40
    Assert-Throws { Publish-TestRelease } '別コミットの下書きを引き継ぎました。'
    Assert-True ('release upload' -cnotin $global:RulecheckReleaseTest_calls) '別コミットの下書きに送信しました。'

    Reset-GitHub
    $global:RulecheckReleaseTest_tagCommit = 'b' * 40
    Assert-Throws { Publish-TestRelease } '別コミットのタグを再利用しました。'
    Assert-True ('release create' -cnotin $global:RulecheckReleaseTest_calls) 'タグの不一致後に作成しました。'

    Reset-GitHub
    $global:RulecheckReleaseTest_failAt = 'api --paginate'
    Assert-Throws { Publish-TestRelease } 'Release一覧の取得失敗を無視しました。'
    Assert-True ('release create' -cnotin $global:RulecheckReleaseTest_calls) '一覧の取得失敗後に作成しました。'

    Write-Output "リリースの条件判定・再実行・公開前の照合を確認しました（$global:RulecheckReleaseTest_assertions 件）。外部公開は行っていません。"
}
finally {
    $resolvedSandbox = [System.IO.Path]::GetFullPath($sandbox)
    $prefix = $temporaryRoot.TrimEnd([System.IO.Path]::DirectorySeparatorChar) + [System.IO.Path]::DirectorySeparatorChar
    if (-not $resolvedSandbox.StartsWith($prefix, [System.StringComparison]::OrdinalIgnoreCase) -or
        [System.IO.Path]::GetFileName($resolvedSandbox) -notlike 'rulecheck-release-test-*') {
        throw '一時領域の範囲を確認できません。'
    }
    Remove-Item -LiteralPath $resolvedSandbox -Recurse -Force
}
