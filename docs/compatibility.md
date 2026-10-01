# Compatibility and release gates

This is an alpha, not completion of the original plan's 1.0 acceptance checklist.

## Contracts

- Claude Code >=2.1.268 for `auth status --json` with `configDirectory`. Local documentation/help inspection: 2.1.285 on Windows.
- Independent `CLAUDE_CONFIG_DIR` and `ANTHROPIC_CONFIG_DIR`; official CLI owns authentication and credential files.
- Statusline usage accepts official `rate_limits.five_hour` and `seven_day`, reported percentage and reset timestamp. Missing/expired data is unknown.
- No cross-account shared transcript directory or promise of cross-account `--resume` compatibility.

## Verification record

| Check | Status |
| --- | --- |
| Windows core, desktop service and CLI fixture-process tests | Passed locally (`go test -count=1 ./...`) |
| Go vet and core source vulnerability scan | Passed; govulncheck reports no known vulnerabilities |
| Windows/Linux/macOS CLI cross-builds | Passed for amd64 and arm64 on all three systems |
| GitHub Actions core checks and six CLI builds | Passed on [commit 9cab233](https://github.com/harveyxiacn/cc-router/actions/runs/36889169668) |
| Windows desktop production build | Passed locally with Wails 2.16.0; native dialog/terminal acceptance remains separate |
| Frontend unit and browser interaction tests | 18 frontend tests passed; browser fixture covers login, handoff and updates |
| Signed updater helper lifecycle | Windows fixture processes: successful startup, failed-startup rollback and restart; no real accounts used |
| Desktop CI on all three OSes | Final workflow verification pending |
| Windows symlink rejection runtime tests | Some tests skip when OS denies creating symlinks |
| Two real Claude accounts per OS, restart persistence | **Not tested**; owner login required |
| Real terminal Ctrl+C/resize/interactive prompts | **Not tested**; fixture process tests are narrower |
| CachyOS native Bash/Zsh/Fish | **Not tested**; Ubuntu CI is not CachyOS acceptance |
| macOS Keychain account isolation | Documented by upstream; **not verified on real accounts** |
| Update manifest signature | Ed25519 verification and SHA-256 archive checks implemented |
| Authenticode, macOS binary signing and notarization | **Not provided in alpha** |
| Cross-account transcript-path resume | **Not implemented or validated** |
| Enterprise MDM/registry/cloud policy completeness | Conservative detection; no full policy-merging implementation |

## Before 1.0

1. Run two real subscriptions on each OS; compare official `/status` email/billing and verify restart isolation.
2. Test terminal signals, resize, Unicode/space paths, cancellation, and descendant/background process lifecycle in actual shells.
3. Test quota warnings with official statusline output; verify staleness and model-specific limits, then perform a manual handoff in a real project.
4. Review managed settings detection against the pinned official version and managed environments; preserve organization policy.
5. Validate restore/migration and uninstall preservation. Implement migration only when there is an actual second schema.
6. Produce signed binaries, macOS notarization, checksums and a release-specific verification record.

## Manual owner smoke test

Use test accounts/profiles created through `ccr account add`, then `ccr login`. Never paste credentials into bug reports. Check both identities with `ccr status` and official `/status`. In a disposable project, run an ordinary task, record its changes/tests in `ccr handoff`, exit, and use `ccr switch OTHER --handoff-reviewed`. Confirm the new conversation can continue from the actual workspace without sharing history.

Uninstall removes binaries, not profiles. Registry removal preserves official data. Review any eventual profile deletion manually and separately.
