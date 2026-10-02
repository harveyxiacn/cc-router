param(
    [Parameter(Mandatory)][string]$Stage,
    [string]$OutputDirectory = (Join-Path (Split-Path $PSScriptRoot -Parent) 'dist'),
    [string]$Version = '0.2.0-beta.1',
    [ValidateSet('amd64', 'arm64')][string]$Architecture = 'amd64',
    [string]$MakeNsis = 'makensis'
)
$ErrorActionPreference = 'Stop'
if ($Version -notmatch '^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$') { throw 'Invalid release version' }
$stagePath = (Resolve-Path -LiteralPath $Stage).Path
# Explicit files only: updater locks, account data and old build outputs are never shipped.
$files = @('cc-router-desktop.exe', 'cc-router.exe', 'README.md', 'LICENSE', 'THIRD_PARTY_NOTICES.txt', 'compatibility.md', 'DESKTOP.md')
foreach ($file in $files) {
    $item = Get-Item -LiteralPath (Join-Path $stagePath $file)
    if ($item.PSIsContainer -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw "Not a regular packaging file: $file" }
}
$compiler = Get-Command $MakeNsis -ErrorAction SilentlyContinue
if (-not $compiler -and $MakeNsis -eq 'makensis') {
    foreach ($candidate in @("${env:ProgramFiles(x86)}/NSIS/makensis.exe", "$env:ProgramFiles/NSIS/makensis.exe")) {
        if (Test-Path -LiteralPath $candidate -PathType Leaf) { $compiler = Get-Command $candidate; break }
    }
}
if (-not $compiler) { throw 'NSIS compiler missing. Install NSIS or pass -MakeNsis <path/to/makensis.exe>.' }
New-Item -ItemType Directory -Path $OutputDirectory -Force | Out-Null
$outputRoot = (Resolve-Path -LiteralPath $OutputDirectory).Path
$output = Join-Path $outputRoot "cc-router-desktop-windows-$Architecture-setup.exe"
$distributionDoc = Join-Path (Split-Path $PSScriptRoot -Parent) 'docs/distribution.md'
& $compiler.Source /V3 "/DSTAGE=$stagePath" "/DOUTPUT=$output" "/DVERSION=$Version" "/DARCH=$Architecture" "/DDISTRIBUTION_DOC=$distributionDoc" (Join-Path $PSScriptRoot 'windows-installer.nsi')
if ($LASTEXITCODE -ne 0) { throw 'Windows installer compilation failed' }
if (-not (Test-Path -LiteralPath $output -PathType Leaf)) { throw 'NSIS did not produce an installer' }
$hash = (Get-FileHash -LiteralPath $output -Algorithm SHA256).Hash.ToLowerInvariant()
"$hash  $([IO.Path]::GetFileName($output))" | Set-Content -LiteralPath ($output + '.sha256') -Encoding ascii
Write-Output $output
