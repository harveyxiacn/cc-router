param(
    [Parameter(Mandatory)][string]$Stage,
    [string]$OutputDirectory = (Join-Path (Split-Path $PSScriptRoot -Parent) 'dist'),
    [string]$Version = '0.2.0-beta.1',
    [ValidateSet('amd64', 'arm64')][string]$Architecture = 'arm64'
)
$ErrorActionPreference = 'Stop'
if ($Version -notmatch '^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$') { throw 'Invalid release version' }
if (-not (Get-Command hdiutil -ErrorAction SilentlyContinue)) { throw 'DMG packaging requires native macOS hdiutil' }
$stagePath = (Resolve-Path -LiteralPath $Stage).Path
$bundles = @(Get-ChildItem -LiteralPath $stagePath -Directory -Filter '*.app')
if ($bundles.Count -ne 1 -or ($bundles[0].Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Expected exactly one regular macOS app bundle' }
foreach ($binary in @('cc-router-desktop', 'cc-router')) {
    $item = Get-Item -LiteralPath (Join-Path $bundles[0].FullName "Contents/MacOS/$binary")
    if ($item.PSIsContainer -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw "Missing regular app executable: $binary" }
}
$files = @('README.md', 'LICENSE', 'THIRD_PARTY_NOTICES.txt', 'compatibility.md', 'DESKTOP.md')
foreach ($file in $files) {
    $item = Get-Item -LiteralPath (Join-Path $stagePath $file)
    if ($item.PSIsContainer -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw "Not a regular packaging file: $file" }
}
New-Item -ItemType Directory -Path $OutputDirectory -Force | Out-Null
$outputRoot = (Resolve-Path -LiteralPath $OutputDirectory).Path
$dmgStage = Join-Path $outputRoot ('.dmg-stage-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $dmgStage | Out-Null
try {
    # ditto preserves the bundle's executable modes and resources.
    & ditto $bundles[0].FullName (Join-Path $dmgStage $bundles[0].Name)
    if ($LASTEXITCODE -ne 0) { throw 'App bundle staging failed' }
    foreach ($file in $files) { Copy-Item -LiteralPath (Join-Path $stagePath $file) -Destination $dmgStage }
    Copy-Item -LiteralPath (Join-Path (Split-Path $PSScriptRoot -Parent) 'docs/distribution.md') -Destination $dmgStage
    & ln -s /Applications (Join-Path $dmgStage 'Applications')
    if ($LASTEXITCODE -ne 0) { throw 'Applications shortcut creation failed' }
    $output = Join-Path $outputRoot "cc-router-desktop-darwin-$Architecture.dmg"
    & hdiutil create -ov -format UDZO -fs HFS+ -volname "CC Router $Version" -srcfolder $dmgStage $output
    if ($LASTEXITCODE -ne 0) { throw 'DMG creation failed' }
    & hdiutil verify $output
    if ($LASTEXITCODE -ne 0) { throw 'DMG verification failed' }
    $hash = (Get-FileHash -LiteralPath $output -Algorithm SHA256).Hash.ToLowerInvariant()
    "$hash  $([IO.Path]::GetFileName($output))" | Set-Content -LiteralPath ($output + '.sha256') -Encoding ascii
    Write-Output $output
} finally {
    # Never recursively remove an unchecked computed path.
    $cleanup = [IO.Path]::GetFullPath($dmgStage)
    $boundary = $outputRoot.TrimEnd([IO.Path]::DirectorySeparatorChar) + [IO.Path]::DirectorySeparatorChar
    if (-not $cleanup.StartsWith($boundary, [StringComparison]::Ordinal) -or [IO.Path]::GetFileName($cleanup) -notmatch '^\.dmg-stage-[a-f0-9]{32}$') { throw 'Unsafe DMG cleanup path' }
    Remove-Item -LiteralPath $cleanup -Recurse -Force
}
