# Desktop downloads and installation

Download desktop assets from this project's [GitHub Releases](https://github.com/harveyxiacn/cc-router/releases).
Each release includes checksums for the installer, disk images and portable archives.

| Platform | Initial installation | Portable / OTA payload |
| --- | --- | --- |
| Windows x64 | `cc-router-desktop-windows-amd64-setup.exe` | `cc-router-desktop-windows-amd64.zip` |
| macOS Apple Silicon | `cc-router-desktop-darwin-arm64.dmg` | `cc-router-desktop-darwin-arm64.tar.gz` |
| macOS Intel | `cc-router-desktop-darwin-amd64.dmg` | `cc-router-desktop-darwin-amd64.tar.gz` |
| Linux x64 | Extract the portable archive | `cc-router-desktop-linux-amd64.tar.gz` |

## Windows

Run the setup executable as your normal Windows user. It installs the desktop app,
companion `cc-router.exe`, license, notices and documentation into
`%LOCALAPPDATA%\Programs\CC Router`, creates Start menu and desktop shortcuts, and
adds a per-user entry to Installed apps. It needs no administrator privileges and
does not change PATH or install the legacy `ccr` alias. The WebView2 runtime is
required; see [Wails' Windows prerequisites](https://wails.io/docs/gettingstarted/installation/).
The setup does not silently download or elevate a WebView2 installer.

Existing users should use the desktop's built-in updater. For a manual reinstall,
close the GUI, companion CLI and managed sessions first. The installer takes the
same exclusive installation lease as OTA, preflights all owned files, stages both
executables before replacement, and restores its previous files if replacement
fails. It never kills a running process or schedules replacement after reboot.
An unfinished or damaged `.cc-router-update-work` journal blocks manual setup
and uninstall; use the app's updater recovery before manual replacement. A
completed or rolled-back OTA journal and its retained program backup are validated
and retired by the packaged companion before manual setup/uninstall. A manual
reinstall therefore replaces the previous OTA rollback point.
A `.cc-router-install-new` or `.cc-router-install-old` directory also blocks another
setup: preserve those directories and recover their files before retrying.
This file transaction handles ordinary copy/rename failures; it does not promise
automatic recovery from power loss during setup. OTA has its own recovery journal.

Uninstall using Installed apps or the Start menu shortcut. Only the package's
known program files and shortcuts are removed. Account profiles, local metadata,
usage records, configuration backups and any custom data root are preserved;
unlisted files keep the installation directory from being removed. A portable
installation in the setup destination is refused unless it was created by setup.

## macOS

Choose the DMG matching your CPU. Open it, drag the `.app` to Applications, then
eject the image. The companion CLI is inside `Contents/MacOS/cc-router`; license,
notices and documentation are inside `Contents/Resources/Documentation` and also
supplied on the disk image. The Applications
item in the disk image is a link to `/Applications`.

OTA requires a writable app location and parent directory. `/Applications` may be
read-only for your normal user; in that case manually replace the app, or use the
portable archive in a directory you own, such as `~/Applications/CC Router/`.
Close the app and all managed sessions before manual replacement. Removing the
app does not remove your account data.

macOS OTA archives contain only the `.app` bundle. Updates replace its internal
files and cannot place generic README/license documents in the shared Applications
directory. The app's parent directory must still be writable for its installation
lease and recovery journal.

## Download trust and OTA signing

These preview setup executables and DMGs do not require a paid OS code-signing
certificate to build or publish. Windows may show an unknown publisher or
SmartScreen warning; macOS Gatekeeper may block an unsigned or unnotarized app.
Use the explicit per-app approval controls described by
[Microsoft](https://learn.microsoft.com/en-us/windows/apps/package-and-deploy/smartscreen-reputation)
and [Apple](https://support.apple.com/102445) after checking the release source.
Do not disable system-wide protection to install a preview.

SHA-256 checksums detect altered downloads but do not identify a publisher when
downloaded from the same source. Ed25519-signed OTA metadata independently
authenticates the portable ZIP/tar.gz payloads against the app's embedded release
key. The installer and DMG are initial-install assets; the updater downloads the
existing portable archives. Ed25519 signing does not provide Windows Authenticode,
macOS Developer ID signing or Apple notarization.

Real certificate signing/notarization is a separate maintainer option. It has not
been tested by this unsigned packaging workflow. In particular, the current
file-based macOS OTA transaction does not support replacing a Developer ID signed
bundle: whole-bundle update/recovery must be implemented before advertising that
combination. See [updates.md](updates.md) for OTA guarantees and release signing.

## Building release packages

On each native platform, with Go, Node and Wails installed:

```powershell
# Windows also requires NSIS (CI installs the pinned compiler with Chocolatey).
./scripts/build-desktop.ps1 -Version 0.2.0-beta.1
# Optional explicit Windows compiler path:
./scripts/build-desktop.ps1 -Version 0.2.0-beta.1 -MakeNsis 'C:/tools/nsis/makensis.exe'
# Archive-only developer build:
./scripts/build-desktop.ps1 -Version 0.2.0-beta.1 -SkipInstallers
```

The script produces the existing OTA archive plus setup/DMG and individual
`.sha256` files. The packaging helpers accept an already-built stage for compile
validation without launching an installer. Only known application files enter the
setup and DMG; account data, updater locks and stale loose files are excluded.

CI builds Windows and Linux x64, macOS ARM64 on `macos-15`, and macOS Intel on
`macos-15-intel`. These native labels are listed in
[GitHub's runner documentation](https://docs.github.com/en/actions/reference/runners/github-hosted-runners).
The Windows setup uses [NSIS](https://nsis.sourceforge.io/Docs/Chapter4.html), as
supported by [Wails 2](https://wails.io/docs/guides/windows-installer/). DMGs use
Apple's native `hdiutil` with a read-only UDZO image; see
[Apple's distribution documentation](https://developer.apple.com/documentation/xcode/packaging-mac-software-for-distribution).

Collect the passing CI artifacts, sign the ZIP/tar.gz OTA descriptors with the
maintainer release key, and publish the setup executables, DMGs, portable archives,
checksums, `cc-router-update.json` and `cc-router-update.sig` together on the same
versioned GitHub Release. Never replace published assets in place. Local Windows
compiler checks do not establish real macOS installation or native installer
upgrade/uninstall acceptance; those require native interactive verification.

## Windows installer runtime notice

The setup and uninstaller embed unmodified NSIS 3.13 runtime components, including
the System, nsExec and nsDialogs plugins, Modern UI artwork, and the zlib
compression engine. Copyright (C) 1999-2026 NSIS Contributors. Their applicable
license is reproduced below from the compiler's COPYING file; see
[NSIS licensing](https://nsis.sourceforge.io/License). The installer uses zlib
compression and does not embed NSIS's separately licensed LZMA or bzip2 modules.

**zlib/libpng license**

This software is provided 'as-is', without any express or implied warranty. In no
event will the authors be held liable for any damages arising from the use of
this software.

Permission is granted to anyone to use this software for any purpose, including
commercial applications, and to alter it and redistribute it freely, subject to
the following restrictions:

1. The origin of this software must not be misrepresented; you must not claim
   that you wrote the original software. If you use this software in a product,
   an acknowledgment in the product documentation would be appreciated but is
   not required.
2. Altered source versions must be plainly marked as such, and must not be
   misrepresented as being the original software.
3. This notice may not be removed or altered from any source distribution.
