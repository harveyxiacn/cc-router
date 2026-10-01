# Automatic update and rollback implementation plan

The owner explicitly selected full background upgrades and rollback. Also add
a default-on option to open official login after creating a local account.
Authentication still belongs entirely to the unmodified official CLI.

## Update contract

- Automatic updates operate while the desktop is running. Check on startup and
  periodically; show a visible setting to disable them and a manual check button.
- Read only this project's public GitHub Releases. Stable installations accept
  stable releases; alpha installations can accept newer prereleases.
- A release contains a bounded JSON manifest and detached Ed25519 signature.
  The app embeds the verification public key. The owner's signing private key
  is generated and protected on the local Windows machine, outside this repo.
  Asset URLs, sizes, SHA-256 hashes, version, platform and archive entrypoints
  are signed. Transport uses HTTPS with fixed GitHub origins and timeouts.
- Download in the background into the private updater cache. Verify signature
  before trusting metadata and verify archive hash before extraction. Reject
  traversal, absolute paths, duplicate entries, links, devices, excessive file
  counts/sizes and unexpected application layouts.
- Install only a writable portable desktop bundle. System package managers,
  read-only locations and unknown layouts require manual installation. No
  elevation or OS security-policy bypass is attempted.
- Wait until managed Claude sessions have ended and UI reports no unsaved
  handoff, active form/dialog or operation. No forced account/process rotation.
- A copied companion CLI runs as the updater helper, outside the files being
  replaced. It waits for desktop exit, rechecks the installation/session lease,
  stages new files on the destination filesystem and preserves old files.
- Launch the new desktop and require an explicit frontend-ready health marker.
  Crash or timeout restores the previous files and restarts the prior version.
  Account profiles, registry, handoffs and usage data are never update targets.
- Record transaction progress before filesystem mutations, preserve recovery
  information after partial failure, and retain the last backup for rollback.

## Work split and verification

Root: update metadata/version parsing, signing and verification, downloader and
safe extraction, installation/helper/recovery, service orchestration, release
signing and CI. GPT-6.1-Sol high: login onboarding, update UI and Wails lifecycle
binding; independent review of the updater when the first implementation exists.

Tests must cover signature/hash failures, release/channel comparison, traversal
and archive bombs, idle protection, failed replacements, startup health failure
and automatic rollback in disposable installation directories. Browser tests
must ensure drafts and active dialogs delay installation. Actual signed GitHub
upgrade and native dialogs on all OSes remain release acceptance evidence to
record separately from unit/fixture tests.
