param(
    [string]$Go = 'go',
    [string]$Wails = 'wails',
    [string]$Version = '0.2.0-beta.1',
    [string]$MakeNsis = 'makensis',
    [switch]$SkipInstallers
)
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
    if ($LASTEXITCODE -ne 0 -or $targetOS -notmatch '^(windows|darwin|linux)$' -or $targetArch -notmatch '^(amd64|arm64)$') { throw 'Unsupported desktop target' }
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
    # Rebuilding cannot carry stale binaries, update locks or loose data into OTA.
    if (Test-Path -LiteralPath $stage) {
        $cleanup = [System.IO.Path]::GetFullPath($stage)
        $distRoot = [System.IO.Path]::GetFullPath((Join-Path $projectRoot 'dist'))
        if ((Get-Item -LiteralPath $distRoot).Attributes -band [System.IO.FileAttributes]::ReparsePoint) { throw 'Desktop output directory cannot be a link' }
        $boundary = $distRoot.TrimEnd([System.IO.Path]::DirectorySeparatorChar) + [System.IO.Path]::DirectorySeparatorChar
        if (-not $cleanup.StartsWith($boundary, [System.StringComparison]::OrdinalIgnoreCase) -or [System.IO.Path]::GetFileName($cleanup) -notmatch '^desktop-(windows|darwin|linux)-(amd64|arm64)$') { throw 'Unsafe desktop staging path' }
        $existingStage = Get-Item -LiteralPath $cleanup
        if ($existingStage.Attributes -band [System.IO.FileAttributes]::ReparsePoint) { throw 'Desktop staging directory cannot be a link' }
        Remove-Item -LiteralPath $cleanup -Recurse -Force
    }
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
    if ($targetOS -eq 'darwin') {
        # Applications is shared by unrelated apps. OTA writes only within ours.
        $bundleDocumentation = Join-Path $stage ($bundles[0].Name + '/Contents/Resources/Documentation')
        New-Item -ItemType Directory -Path $bundleDocumentation -Force | Out-Null
        foreach ($file in @('README.md', 'LICENSE', 'THIRD_PARTY_NOTICES.txt', 'compatibility.md', 'DESKTOP.md')) {
            Copy-Item -LiteralPath (Join-Path $stage $file) -Destination $bundleDocumentation -Force
        }
        Copy-Item -LiteralPath 'docs/distribution.md' -Destination $bundleDocumentation -Force
        $appPath = Join-Path $stage $bundles[0].Name
        # Go 1.27 targets macOS 13; replace Wails' older template declaration.
        & /usr/bin/plutil -replace LSMinimumSystemVersion -string '13.0' (Join-Path $appPath 'Contents/Info.plist')
        if ($LASTEXITCODE -ne 0) { throw 'macOS minimum system version declaration failed' }
        # Wails ad-hoc signs the main bundle before we add the companion/docs.
        # Seal the final layout; '-' provides integrity, not Developer ID trust.
        & /usr/bin/codesign --force --sign - $cliPath
        if ($LASTEXITCODE -ne 0) { throw 'Companion ad-hoc signing failed' }
        & /usr/bin/codesign --force --sign - $appPath
        if ($LASTEXITCODE -ne 0) { throw 'Final app ad-hoc signing failed' }
        & /usr/bin/codesign --verify --deep --strict --verbose=2 $appPath
        if ($LASTEXITCODE -ne 0) { throw 'Final app ad-hoc signature verification failed' }
    }
    if ($targetOS -eq 'windows') {
        $archive = Join-Path $projectRoot "dist/cc-router-desktop-$targetOS-$targetArch.zip"
        Compress-Archive -Path (Join-Path $stage '*') -DestinationPath $archive -Force
    } else {
        $archive = Join-Path $projectRoot "dist/cc-router-desktop-$targetOS-$targetArch.tar.gz"
        if ($targetOS -eq 'darwin') {
            & tar -czf $archive -C $stage $bundles[0].Name
        } else {
            & tar -czf $archive -C $stage .
        }
        if ($LASTEXITCODE -ne 0) { throw 'Desktop archive failed' }
    }
    $hash = (Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant()
    "$hash  $([System.IO.Path]::GetFileName($archive))" | Set-Content -LiteralPath ($archive + '.sha256') -Encoding ascii
    if (-not $SkipInstallers) {
        if ($targetOS -eq 'windows') {
            & (Join-Path $PSScriptRoot 'package-windows.ps1') -Stage $stage -Version $Version -Architecture $targetArch -MakeNsis $MakeNsis
        } elseif ($targetOS -eq 'darwin') {
            & (Join-Path $PSScriptRoot 'package-macos.ps1') -Stage $stage -Version $Version -Architecture $targetArch
        }
    }
    Write-Output "Built desktop $targetOS-$targetArch"
} finally {
    [System.IO.File]::WriteAllBytes($configurationPath, $originalConfiguration)
    $env:GOFLAGS = $previousGoFlags
    Pop-Location
}
