param([Parameter(Mandatory)][string]$Installer)
$ErrorActionPreference = 'Stop'
# This script runs actual setup/uninstall. Only the hosted CI VM is disposable.
if ($env:GITHUB_ACTIONS -ne 'true' -or $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or $env:RUNNER_OS -ne 'Windows') {
    throw 'Installer lifecycle smoke tests may run only on a disposable GitHub-hosted Windows runner.'
}
$installerPath = (Resolve-Path -LiteralPath $Installer).Path
$installRoot = Join-Path ([Environment]::GetFolderPath('LocalApplicationData')) 'Programs/CC Router'
if (Test-Path -LiteralPath $installRoot) { throw 'Smoke test requires an empty per-user installation location.' }
$uninstallKey = 'HKCU:/Software/Microsoft/Windows/CurrentVersion/Uninstall/CC Router'
$startMenuRoot = Join-Path ([Environment]::GetFolderPath('Programs', [Environment+SpecialFolderOption]::DoNotVerify)) 'CC Router'
$desktopShortcut = Join-Path ([Environment]::GetFolderPath('DesktopDirectory', [Environment+SpecialFolderOption]::DoNotVerify)) 'CC Router.lnk'

function Invoke-Setup([string]$Executable, [bool]$ExpectSuccess, [string[]]$Arguments = @('/S')) {
    $process = Start-Process -FilePath $Executable -ArgumentList $Arguments -WindowStyle Hidden -PassThru
    if (-not $process.WaitForExit(45000)) { $process.Kill(); throw 'Installer smoke test timed out.' }
    $process.Refresh()
    if (($process.ExitCode -eq 0) -ne $ExpectSuccess) { throw "Unexpected setup exit $($process.ExitCode), expected success=$ExpectSuccess" }
}
function Get-BinaryHashes {
    return @('cc-router-desktop.exe', 'cc-router.exe') | ForEach-Object { (Get-FileHash -LiteralPath (Join-Path $installRoot $_)).Hash }
}
function Assert-OldBinaries([string[]]$Expected) {
    if (((Get-BinaryHashes) -join ',') -ne ($Expected -join ',')) { throw 'A refused installation changed the old binaries.' }
}
function New-TerminalJournal([string]$Phase) {
    $id = [Guid]::NewGuid().ToString('N')
    $transactionRoot = Join-Path $installRoot ".cc-router-update-work/$id"
    New-Item -ItemType Directory -Path (Join-Path $transactionRoot 'backup'), (Join-Path $transactionRoot 'new') -Force | Out-Null
    $readmePath = Join-Path $installRoot 'README.md'
    Copy-Item -LiteralPath $readmePath -Destination (Join-Path $transactionRoot 'backup/README.md')
    $descriptor = @{path = 'README.md'; sha256 = (Get-FileHash -LiteralPath $readmePath).Hash.ToLowerInvariant(); size = (Get-Item -LiteralPath $readmePath).Length; mode = 420}
    $journal = @{schema = 1; id = $id; phase = $Phase; entries = @(@{new = $descriptor; old = $descriptor})} | ConvertTo-Json -Depth 8
    [IO.File]::WriteAllText((Join-Path $transactionRoot 'journal.json'), $journal, [Text.UTF8Encoding]::new($false))
}

Invoke-Setup $installerPath $true
foreach ($file in @('cc-router-desktop.exe', 'cc-router.exe', 'LICENSE', 'THIRD_PARTY_NOTICES.txt', 'README.md', 'DESKTOP.md', 'compatibility.md', 'distribution.md', 'Uninstall.exe', '.cc-router-installed')) {
    if (-not (Test-Path -LiteralPath (Join-Path $installRoot $file) -PathType Leaf)) { throw "Missing installed file: $file" }
}
if (-not (Test-Path -LiteralPath $uninstallKey)) { throw 'Per-user Installed apps registration missing.' }
if (-not (Test-Path -LiteralPath $desktopShortcut) -or -not (Test-Path -LiteralPath (Join-Path $startMenuRoot 'CC Router.lnk'))) { throw 'User shortcuts missing.' }
$oldHashes = @(Get-BinaryHashes)
# _?= suppresses NSIS's bootstrap child so WaitForExit observes the actual
# uninstaller result. Run a copied uninstaller so its installed file is unlocked.
$uninstallerCopy = Join-Path $env:RUNNER_TEMP 'cc-router-uninstall-smoke.exe'
Copy-Item -LiteralPath (Join-Path $installRoot 'Uninstall.exe') -Destination $uninstallerCopy
$dataSentinelRoot = Join-Path $installRoot 'smoke-account-data'
New-Item -ItemType Directory -Path $dataSentinelRoot | Out-Null
[IO.File]::WriteAllText((Join-Path $dataSentinelRoot 'preserve.txt'), 'preserve-account-data')

# Use the same byte-zero lease as GUI/CLI/OTA, without launching a desktop session.
$lease = [IO.File]::Open((Join-Path $installRoot '.cc-router-update.lock'), [IO.FileMode]::OpenOrCreate, [IO.FileAccess]::ReadWrite, [IO.FileShare]::ReadWrite)
try {
    $lease.Lock(0, 1)
    Invoke-Setup $installerPath $false
    Assert-OldBinaries $oldHashes
    Invoke-Setup $uninstallerCopy $false @('/S', "_?=$installRoot")
    Assert-OldBinaries $oldHashes
} finally { $lease.Dispose() }

# Also cover preflight failure for older binaries that do not hold an OTA lease.
foreach ($binary in @('cc-router-desktop.exe', 'cc-router.exe')) {
    $lockedFile = [IO.File]::Open((Join-Path $installRoot $binary), [IO.FileMode]::Open, [IO.FileAccess]::Read, [IO.FileShare]::None)
    try {
        Invoke-Setup $installerPath $false
    } finally { $lockedFile.Dispose() }
    Assert-OldBinaries $oldHashes
}

# A damaged OTA journal must block manual setup/uninstall without losing evidence.
$journalRoot = Join-Path $installRoot '.cc-router-update-work'
New-Item -ItemType Directory -Path $journalRoot | Out-Null
[IO.File]::WriteAllText((Join-Path $journalRoot 'journal.json'), '{invalid-smoke-journal')
Invoke-Setup $installerPath $false
Assert-OldBinaries $oldHashes
Invoke-Setup $uninstallerCopy $false @('/S', "_?=$installRoot")
Assert-OldBinaries $oldHashes
if ([IO.File]::ReadAllText((Join-Path $journalRoot 'journal.json')) -ne '{invalid-smoke-journal') { throw 'Refused setup changed the damaged journal.' }
Remove-Item -LiteralPath (Join-Path $journalRoot 'journal.json')
Remove-Item -LiteralPath $journalRoot

New-TerminalJournal 'complete'
Invoke-Setup $installerPath $true
Assert-OldBinaries $oldHashes
if (Test-Path -LiteralPath (Join-Path $installRoot '.cc-router-update-work')) { throw 'Reinstall did not retire completed OTA state.' }
New-TerminalJournal 'rolled-back'
# A copied uninstaller with a redirected _?= argument must still use its fixed
# per-user installation, leaving similarly named documents in another root alone.
$decoyRoot = Join-Path $dataSentinelRoot 'other-folder'
New-Item -ItemType Directory -Path $decoyRoot | Out-Null
[IO.File]::WriteAllText((Join-Path $decoyRoot 'README.md'), 'preserve-unrelated-readme')
[IO.File]::WriteAllText((Join-Path $decoyRoot 'LICENSE'), 'preserve-unrelated-license')
Copy-Item -LiteralPath (Join-Path $installRoot 'Uninstall.exe') -Destination $uninstallerCopy
Invoke-Setup $uninstallerCopy $true @('/S', "_?=$decoyRoot")
if (Test-Path -LiteralPath (Join-Path $installRoot '.cc-router-update-work')) { throw 'Uninstall did not retire rolled-back OTA state.' }
foreach ($file in @('cc-router-desktop.exe', 'cc-router.exe', 'Uninstall.exe', '.cc-router-installed')) {
    if (Test-Path -LiteralPath (Join-Path $installRoot $file)) { throw "Uninstaller left program file: $file" }
}
if (Test-Path -LiteralPath $uninstallKey) { throw 'Uninstaller left its registry entry.' }
if ((Test-Path -LiteralPath $desktopShortcut) -or (Test-Path -LiteralPath $startMenuRoot)) { throw 'Uninstaller left user shortcuts.' }
if ([IO.File]::ReadAllText((Join-Path $dataSentinelRoot 'preserve.txt')) -ne 'preserve-account-data') { throw 'Uninstaller changed account-data sentinel.' }
if ([IO.File]::ReadAllText((Join-Path $decoyRoot 'README.md')) -ne 'preserve-unrelated-readme' -or [IO.File]::ReadAllText((Join-Path $decoyRoot 'LICENSE')) -ne 'preserve-unrelated-license') { throw 'Uninstaller followed a redirected target.' }
Invoke-Setup $uninstallerCopy $false @('/S', "_?=$decoyRoot")
Remove-Item -LiteralPath $uninstallerCopy
Write-Output 'Installer install, busy/locked/corrupt-update refusal, reinstall and data-preserving uninstall smoke checks passed.'
