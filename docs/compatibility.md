# Compatibility and release gates

The 0.2 beta adds planned product modules. It is not completion of the original plan's real-account/real-device acceptance checklist. OS code signing is an optional distribution improvement, not a prerequisite for installer/DMG downloads or signed-manifest OTA.

Beta release assets must come from a successful CI run for that version's commit.
The exact commit, CI run and published checksums are recorded on the
[0.2.0-beta.1 release](https://github.com/harveyxiacn/cc-router/releases/tag/v0.2.0-beta.1);
the configured checks are visible in the [CI workflow](https://github.com/harveyxiacn/cc-router/actions/workflows/ci.yml).

## Contracts

- Claude Code >=2.1.268 for `auth status --json` with `configDirectory`. Local documentation/help inspection: 2.1.285 on Windows.
- Independent `CLAUDE_CONFIG_DIR` and `ANTHROPIC_CONFIG_DIR`; official CLI owns authentication and credential files.
- Statusline usage accepts official `rate_limits.five_hour` and `seven_day`, reported percentage and reset timestamp. Missing/expired data is unknown.
- No cross-account shared transcript directory or promise of cross-account `--resume` compatibility.

## Verification record

| Check | Status |
| --- | --- |
| Windows core, desktop service and CLI fixture-process tests | Passed locally in the beta worktree; release-commit CI must also pass |
| Compiled CLI manual/backup flow | Isolated temporary-data record, backup, rename, reviewed restore and clear loop passed; no account login used |
| Go vet and source vulnerability checks | Local checks passed; release-commit CI must also pass |
| Windows/Linux/macOS CLI cross-builds | Six-target build configured (amd64 and arm64 on each OS); publish only successful release-commit CI artifacts |
| GitHub Actions core checks | Required for release: updater lifecycle, Linux race/vulnerability checks and repeated macOS lock regressions; earlier alpha results are not beta evidence |
| Windows desktop production build | Built locally with Wails 2.16.0; native dialog/terminal acceptance remains separate |
| Frontend unit and browser interaction tests | **27 frontend tests passed**, TypeScript/Vite build passed; browser fixture passed login onboarding, separate manual/official usage, backup creation/preview/reviewed restore, handoff conflicts and update controls |
| Signed updater helper lifecycle | Fixture coverage for startup confirmation, failed-startup rollback/restart and manual rollback; release requires three-OS CI, no real accounts used |
| Desktop CI and release architectures | Four native builds configured: Windows amd64, Linux amd64, macOS arm64 and **macOS amd64 (Intel)**. Release requires successful native build/installer smoke/DMG inspection; artifact availability is recorded on the release page |
| Initial-install packages | Windows per-user setup EXE and both macOS DMGs implemented; release requires native CI. Linux uses the portable archive; installers/DMGs are separate from OTA ZIP/tar.gz payloads |
| Windows symlink rejection runtime tests | Some tests skip when OS denies creating symlinks |
| Two real Claude accounts per OS, restart persistence | **Not tested**; owner login required |
| Real terminal Ctrl+C/resize/interactive prompts | **Not tested**; fixture process tests are narrower |
| CachyOS native Bash/Zsh/Fish | **Not tested**; Ubuntu CI is not CachyOS acceptance |
| macOS Keychain account isolation | Documented by upstream; **not verified on real accounts** |
| Update manifest signature | Ed25519 verification and SHA-256 archive checks implemented |
| Authenticode, macOS Developer ID signing and notarization | **Not provided for this beta**; optional maintainer work, not a gate for the selected unsigned EXE/DMG and Ed25519-signed OTA distribution |
| Cross-account transcript-path resume | **Not implemented or validated** |
| Original plan's manual quota and local limit records | Implemented separately from official statusline observations, with source/time/expiry and explicit clear action |
| Local configuration backup and restore | Implemented with stable-ID/project/default restoration, preview digest, pre-restore and pre-update backup, busy-session checks; schema v1 remains compatible |
| Enterprise MDM/registry/cloud policy completeness | Conservative detection; no full policy-merging implementation |

## Before 1.0

macOS concurrent first lock creation uses exclusive-create followed by an ordinary
open when the lock already exists, avoiding the Darwin `O_CREAT` race documented
in [Go issue 81246](https://github.com/golang/go/issues/81246). CI repeats the
concurrent thread/process regressions on macOS; lock files remain persistent.

1. Run two real subscriptions on each OS; compare official `/status` email/billing and verify restart isolation.
2. Test terminal signals, resize, Unicode/space paths, cancellation, and descendant/background process lifecycle in actual shells.
3. Test quota warnings with official statusline output; verify staleness and model-specific limits, then perform a manual handoff in a real project.
4. Review managed settings detection against the pinned official version and managed environments; preserve organization policy.
5. Validate restore/migration and uninstall preservation. Implement migration only when there is an actual second schema.
6. Produce checksums and a release-specific verification record. Optional Windows Authenticode/macOS notarization requires maintainer signing credentials; unsigned installer and DMG distribution remains supported as selected by the owner.

## Manual owner smoke test

Use test accounts/profiles created through `cc-router account add`, then `cc-router login`. Never paste credentials into bug reports. Check both identities with `cc-router status` and official `/status`. In a disposable project, run an ordinary task, record its changes/tests in `cc-router handoff`, exit, and use `cc-router switch OTHER --handoff-reviewed`. Confirm the new conversation can continue from the actual workspace without sharing history.

Uninstall removes binaries, not profiles. Registry removal preserves official data. Review any eventual profile deletion manually and separately.
