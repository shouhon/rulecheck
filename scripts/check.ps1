[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$projectRoot = Split-Path -Parent $PSScriptRoot
Push-Location -LiteralPath $projectRoot
try {
    $unformatted = @(& gofmt -l cmd internal)
    if ($LASTEXITCODE -ne 0) {
        throw 'Goの書式検査に失敗しました。'
    }
    if ($unformatted.Count -ne 0) {
        throw "書式を整えてください: $($unformatted -join ', ')"
    }

    & go mod tidy -diff
    if ($LASTEXITCODE -ne 0) {
        throw 'go.modまたはgo.sumの整合性を確認してください。'
    }

    & go vet ./...
    if ($LASTEXITCODE -ne 0) {
        throw 'Goの静的検査に失敗しました。'
    }

    & go test ./...
    if ($LASTEXITCODE -ne 0) {
        throw 'Goのテストに失敗しました。'
    }

    & go build -trimpath -o bin/rulecheck.exe ./cmd/rulecheck
    if ($LASTEXITCODE -ne 0) {
        throw 'Windows向け実行ファイルのビルドに失敗しました。'
    }

    & ./bin/rulecheck.exe --version
    if ($LASTEXITCODE -ne 0) {
        throw '実行ファイルの起動確認に失敗しました。'
    }

    & pwsh -NoProfile -File ./scripts/test-release.ps1
    if ($LASTEXITCODE -ne 0) {
        throw 'リリースの条件判定・公開手順のテストに失敗しました。'
    }

    Write-Output '書式・モジュール・静的検査・テスト・ビルド・起動確認が完了しました。'
}
finally {
    Pop-Location
}
