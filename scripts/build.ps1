param([string]$Go = 'go', [string]$Version = '0.1.0-alpha.1', [switch]$IncludeLegacyAlias)
$ErrorActionPreference = 'Stop'
if ($Version -notmatch '^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$') { throw 'Invalid release version' }
$projectRoot = Split-Path $PSScriptRoot -Parent
Push-Location -LiteralPath $projectRoot
$oldOS, $oldArch, $oldCGO = $env:GOOS, $env:GOARCH, $env:CGO_ENABLED
try {
    New-Item -ItemType Directory -Path 'dist' -Force | Out-Null
    $archives = @()
    foreach ($target in @('windows-amd64', 'windows-arm64', 'linux-amd64', 'linux-arm64', 'darwin-amd64', 'darwin-arm64')) {
        $parts = $target.Split('-')
        $env:GOOS, $env:GOARCH, $env:CGO_ENABLED = $parts[0], $parts[1], '0'
        $stage = Join-Path 'dist/staging' $target
        New-Item -ItemType Directory -Path $stage -Force | Out-Null
        $suffix = if ($parts[0] -eq 'windows') { '.exe' } else { '' }
        $binary = Join-Path $stage ('cc-router' + $suffix)
        & $Go build -buildvcs=false -trimpath -ldflags "-s -w -X github.com/harveyxiacn/cc-router/internal/cli.Version=$Version" -o $binary ./cmd/ccr
        if ($LASTEXITCODE -ne 0) { throw "Build failed: $target" }
        $packageFiles = @(('cc-router' + $suffix), 'README.md', 'LICENSE', 'THIRD_PARTY_NOTICES.txt', 'compatibility.md')
        if ($IncludeLegacyAlias) {
            $aliasName = 'ccr' + $suffix
            Copy-Item -LiteralPath $binary -Destination (Join-Path $stage $aliasName) -Force
            $packageFiles += $aliasName
        }
        Copy-Item -LiteralPath 'README.md', 'LICENSE', 'THIRD_PARTY_NOTICES.txt', 'docs/compatibility.md' -Destination $stage -Force
        if ($parts[0] -eq 'windows') {
            $archive = Join-Path 'dist' ("cc-router-$target.zip")
            $packagePaths = @($packageFiles | ForEach-Object { Join-Path $stage $_ })
            Compress-Archive -LiteralPath $packagePaths -DestinationPath $archive -Force
        } else {
            $archive = Join-Path 'dist' ("cc-router-$target.tar.gz")
            # Windows-created archives may need chmod +x after extraction, as documented.
            & tar -czf $archive --options 'gzip:compression-level=9' -C $stage @packageFiles
            if ($LASTEXITCODE -ne 0) { throw "Archive failed: $target" }
        }
        $archives += $archive
        Write-Output "Built $target"
    }
    $lines = foreach ($archive in $archives) {
        $hash = (Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant()
        "$hash  $([System.IO.Path]::GetFileName($archive))"
    }
    $lines | Set-Content -LiteralPath 'dist/SHA256SUMS' -Encoding ascii
} finally {
    $env:GOOS, $env:GOARCH, $env:CGO_ENABLED = $oldOS, $oldArch, $oldCGO
    Pop-Location
}
