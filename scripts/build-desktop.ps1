param([string]$Go = 'go', [string]$Wails = 'wails', [string]$Version = '0.1.0-alpha.1')
$ErrorActionPreference = 'Stop'
if ($Version -notmatch '^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$') { throw 'Invalid release version' }
$projectRoot = Split-Path $PSScriptRoot -Parent
$previousGoFlags = $env:GOFLAGS
$configurationPath = Join-Path $projectRoot 'desktop/wails.json'
$originalConfiguration = [System.IO.File]::ReadAllBytes($configurationPath)
Push-Location -LiteralPath $projectRoot
try {
    $targetOS = (& $Go env GOOS).Trim()
    $targetArch = (& $Go env GOARCH).Trim()
    $env:GOFLAGS = '-buildvcs=false'
    $configuration = [System.Text.Encoding]::UTF8.GetString($originalConfiguration) | ConvertFrom-Json
    if ($null -eq $configuration.info) {
        $configuration | Add-Member -NotePropertyName info -NotePropertyValue ([PSCustomObject]@{})
    }
    $configuration.info | Add-Member -NotePropertyName productVersion -NotePropertyValue $Version.Split('-')[0] -Force
    $configuration.info | Add-Member -NotePropertyName comments -NotePropertyValue "CC Router $Version" -Force
    $configurationText = ($configuration | ConvertTo-Json -Depth 20) + [Environment]::NewLine
    [System.IO.File]::WriteAllText($configurationPath, $configurationText, [System.Text.UTF8Encoding]::new($false))
    Push-Location -LiteralPath 'desktop'
    try {
        $buildArgs = @('build', '-clean', '-trimpath', '-ldflags', "-s -w -X github.com/harveyxiacn/cc-router/internal/cli.Version=$Version")
        if ($targetOS -eq 'linux') { $buildArgs += @('-tags', 'webkit2_41') }
        & $Wails @buildArgs
        if ($LASTEXITCODE -ne 0) { throw 'Desktop build failed' }
    } finally { Pop-Location }
    $stage = Join-Path $projectRoot "dist/desktop-$targetOS-$targetArch"
    New-Item -ItemType Directory -Path $stage -Force | Out-Null
    if ($targetOS -eq 'darwin') {
        $bundles = @(Get-ChildItem -LiteralPath 'desktop/build/bin' -Directory -Filter '*.app')
        if ($bundles.Count -ne 1) { throw 'Expected one macOS app bundle' }
        Copy-Item -LiteralPath $bundles[0].FullName -Destination $stage -Recurse -Force
        $cliPath = Join-Path $stage ($bundles[0].Name + '/Contents/MacOS/cc-router')
    } else {
        $suffix = if ($targetOS -eq 'windows') { '.exe' } else { '' }
        Copy-Item -LiteralPath ('desktop/build/bin/cc-router-desktop' + $suffix) -Destination $stage -Force
        $cliPath = Join-Path $stage ('cc-router' + $suffix)
    }
    & $Go build -buildvcs=false -trimpath -ldflags "-s -w -X github.com/harveyxiacn/cc-router/internal/cli.Version=$Version" -o $cliPath ./cmd/ccr
    if ($LASTEXITCODE -ne 0) { throw 'Companion CLI build failed' }
    Copy-Item -LiteralPath 'README.md', 'LICENSE', 'THIRD_PARTY_NOTICES.txt', 'docs/compatibility.md' -Destination $stage -Force
    Copy-Item -LiteralPath 'desktop/README.md' -Destination (Join-Path $stage 'DESKTOP.md') -Force
    if ($targetOS -eq 'windows') {
        $archive = Join-Path $projectRoot "dist/cc-router-desktop-$targetOS-$targetArch.zip"
        Compress-Archive -Path (Join-Path $stage '*') -DestinationPath $archive -Force
    } else {
        $archive = Join-Path $projectRoot "dist/cc-router-desktop-$targetOS-$targetArch.tar.gz"
        & tar -czf $archive -C $stage .
        if ($LASTEXITCODE -ne 0) { throw 'Desktop archive failed' }
    }
    $hash = (Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant()
    "$hash  $([System.IO.Path]::GetFileName($archive))" | Set-Content -LiteralPath ($archive + '.sha256') -Encoding ascii
    Write-Output "Built desktop $targetOS-$targetArch"
} finally {
    [System.IO.File]::WriteAllBytes($configurationPath, $originalConfiguration)
    $env:GOFLAGS = $previousGoFlags
    Pop-Location
}
