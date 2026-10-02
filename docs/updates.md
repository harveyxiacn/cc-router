# Desktop updates and release signing

The owner selected full background updating and rollback. Updates run while the
desktop is open; there is no OS service, scheduled task or elevated installer.
Automatic checking/downloading defaults on and can be disabled in the desktop.
Startup and six-hour checks use only this project's public GitHub Releases, with
no GitHub token or account credentials. Manual checks work with automatic updates off.

## Installation transaction

1. Find a newer semantic version on the current channel. Stable builds ignore
   prereleases; alpha builds may receive newer prereleases or stable releases.
2. Verify `cc-router-update.json` against its detached `cc-router-update.sig`
   using the embedded Ed25519 key. Check expiry, platform, entrypoints and version.
3. Download the signed asset into the private local updater cache. Check its exact
   size and SHA-256; extraction rejects links, traversal, duplicate paths, unsafe
   Windows names, unexpected root files and excessive sizes/counts.
4. Wait for no unsaved draft, open dialog, operation or managed Claude session,
   plus 30 seconds without interaction. The explicit manual restart button does
   not require that 30-second delay. No Claude process is killed to make room.
5. Copy the installed companion CLI outside the installation. This helper waits
   for the GUI to exit only after acknowledging verified inputs; a helper startup
   failure keeps the GUI open. It obtains an exclusive installation lease and independently
   verifies the manifest/archive again. GUI and ordinary companion CLI processes
   hold shared installation leases, including while a managed session runs.
6. Save a validated local account/project configuration backup with `pre-update`
   provenance. If metadata cannot be preserved, keep the old application and stop.
   Back up every replaced file before mutating the installation. Record progress
   under `.cc-router-update-work`, then replace the GUI, companion CLI and shipped
   documentation/resources. Account profiles, metadata, usage and handoffs are
   not update targets.
7. Launch the new GUI with a transaction-specific startup probe. A rendered UI
   and successful local snapshot are required before it acknowledges readiness.
   It must report the expected version, nonce and child PID within 45 seconds.
   A crash or timeout restores backups and restarts the previous program.
8. Retain the latest backup for manual rollback. A later successful update retires
   the previous completed backup. Failed automatic versions require an explicit
   manual retry; another desktop blocking installation also suppresses automatic
   retry to avoid a restart loop. A pending transaction is recovered on startup;
   invalid recovery records block managed launches and preserve the evidence.

The cache is in `<data-root>/updates`; backups and the installation lock are next
to the portable app. Keep these files until the update is resolved. Manual rollback
checks that installed files still match the completed transaction before restoring
them. Restoring app binaries does not downgrade account data; this alpha only uses
metadata schema v1. Future schema changes need an explicit migration/rollback policy.

## Supported scope

- Writable, fixed-location portable installations on Windows, Linux and macOS.
  Read-only directories, package-manager installations and unknown layouts require
  manual replacement. The updater does not request administrator/root privileges.
- macOS alpha bundles are unsigned. Updates replace listed files and retain older
  unlisted resources. Signed/notarized bundle updates require a complete bundle
  replacement strategy before those distributions can be supported.
- A startup health check verifies local UI readiness, not real account login,
  terminal interaction or every application feature. Those need native acceptance.
- The journal covers interrupted process recovery. File synchronization and the
  journal do not guarantee recovery from every filesystem failure or sudden power
  loss. Preserve normal backups of your project and local metadata.
- Signed metadata provides update authenticity. It is separate from initial
  download trust, Windows Authenticode and macOS signing/notarization.

The design borrows threat categories from [TUF's security documentation](https://theupdateframework.io/docs/security/)
but is not a TUF implementation. Wails 3's [self-update facilities](https://v3.wails.io/tutorials/04-self-update-a-wails-app/)
were reviewed; this project remains on Wails 2 and updates its two executables
together through a standard-library Go helper. The initial protocol has one
embedded release key, signed expiry/version checks and fixed HTTPS GitHub origins.
It has no threshold signature or root-key rotation protocol.

## Maintainer release workflow

Run from the repository root, using the same version for all builds and the manifest:

```powershell
./scripts/build.ps1 -Version 0.1.0-alpha.1
./scripts/build-desktop.ps1 -Version 0.1.0-alpha.1
# Collect the exact, passing CI desktop artifacts from the other operating systems.
go run ./cmd/release-sign -version 0.1.0-alpha.1 -assets dist -notes release-notes.txt
```

The signing command verifies every archive by extracting it to a disposable
directory before signing. It signs only desktop packages matching the expected
platform naming convention. Publish the archives, checksums, manifest and detached
signature together under the exact `v<version>` tag. Never replace a version's
assets after publication. Archive descriptors bind both executable paths and the
entire compressed payload, including documentation.

`cmd/release-key` provisions a maintainer key once and exports only its public key
to `internal/update/release-public-key.txt`. On Windows the private key is in
`%APPDATA%/cc-router-maintainer/release-signing/private-key.protected`, protected
by current-user DPAPI and directory ACLs. It is not in the repository or GitHub
Actions. Unix maintainer storage uses private directory/file permissions, without
DPAPI. Do not copy the protected Windows file and assume it works under another
user or machine. Protect and back up the maintainer OS profile appropriately;
loss of the key requires a separately planned trust/key migration. The commands
refuse silent rotation when the stored public key differs.

The helper lifecycle tests use an ephemeral test key inside the Go test executable;
production executables have no environment setting to override the release key.
