param(
    [Parameter(Mandatory)][string]$Dmg,
    [ValidateSet('amd64', 'arm64')][string]$Architecture = 'arm64'
)
$ErrorActionPreference = 'Stop'
if (-not (Get-Command hdiutil -ErrorAction SilentlyContinue)) { throw 'DMG inspection requires native macOS hdiutil' }
$dmgPath = (Resolve-Path -LiteralPath $Dmg).Path
$parent = Split-Path $dmgPath -Parent
$mount = Join-Path $parent ('.dmg-check-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $mount | Out-Null
$attached = $false
try {
    & hdiutil attach -readonly -nobrowse -mountpoint $mount $dmgPath
    if ($LASTEXITCODE -ne 0) { throw 'DMG mount failed' }
    $attached = $true
    $bundles = @(Get-ChildItem -LiteralPath $mount -Directory -Filter '*.app')
    if ($bundles.Count -ne 1) { throw 'DMG must contain exactly one app bundle' }
    $expected = @($bundles[0].Name, 'Applications', 'README.md', 'LICENSE', 'THIRD_PARTY_NOTICES.txt', 'compatibility.md', 'DESKTOP.md', 'distribution.md')
    $actual = @(Get-ChildItem -LiteralPath $mount | ForEach-Object Name)
    if (Compare-Object $expected $actual) { throw 'DMG contains unexpected or missing visible root files' }
    $applicationsTarget = & readlink (Join-Path $mount 'Applications')
    if ($LASTEXITCODE -ne 0 -or $applicationsTarget -ne '/Applications') { throw 'DMG Applications link is incorrect' }
    $expectedCpu = if ($Architecture -eq 'amd64') { 'x86_64' } else { 'arm64' }
    foreach ($binary in @('cc-router-desktop', 'cc-router')) {
        $path = Join-Path $bundles[0].FullName "Contents/MacOS/$binary"
        & /usr/bin/test -x $path
        if ($LASTEXITCODE -ne 0) { throw "DMG executable mode missing: $binary" }
        $description = & file -b $path
        if ($LASTEXITCODE -ne 0 -or $description -notmatch ('Mach-O.*' + $expectedCpu)) { throw "DMG CPU mismatch: $binary ($description)" }
    }
    foreach ($document in @('README.md', 'LICENSE', 'THIRD_PARTY_NOTICES.txt', 'compatibility.md', 'DESKTOP.md', 'distribution.md')) {
        if (-not (Test-Path -LiteralPath (Join-Path $bundles[0].FullName "Contents/Resources/Documentation/$document") -PathType Leaf)) { throw "App is missing bundled documentation: $document" }
    }
    Write-Output "DMG layout, Applications link and $Architecture executable checks passed."
} finally {
    if ($attached) {
        & hdiutil detach $mount
        if ($LASTEXITCODE -ne 0) { throw 'Could not detach DMG inspection mount' }
    }
    # No recursive deletion; this is the exact empty mount directory just created.
    Remove-Item -LiteralPath $mount
}
