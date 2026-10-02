# Complete planned modules

**Goal:** Complete the original Word plan's product modules plus the owner's accepted GUI, login onboarding, quota warnings and OTA requirements; retain explicit account switching.

**Architecture:** Extend the existing Go core and Wails GUI. Manual observations remain separate from official observations. Local configuration recovery preserves stable account IDs without reading or replacing official profiles. Existing schema v1 remains readable; an unknown future schema is rejected without mutation. Release engineering covers both macOS CPU architectures and optional OS signing.

**Tech stack:** Go standard library, Wails 2, React/TypeScript, existing GitHub Actions matrix. GPT-6.1-Sol high handles state/recovery and frontend work with root integration and reciprocal review.

## Scope and acceptance

| Module | Implementation work | Evidence |
| --- | --- | --- |
| Primary CLI name | All examples/help use cc-router; legacy ccr packaging opt-in | CLI tests, archive inspection; committed f87ac34 |
| Manual quota observations | Separate manual 5h/7d values, reset time, limited-until marker, record time, clear action; CLI + GUI | Invalid input, expiry, no replacement of official data, persistence tests |
| Configuration recovery | Local named metadata backups, list/preview, explicit restore confirmation, automatic pre-restore backup; preserve IDs/bindings and profiles | Busy-account rejection, unknown schema/corruption, path containment, no profile reads/writes, restore round trip |
| Schema compatibility | Keep schema v1; validate backup envelopes and fail closed on unknown schemas; recovery can replace a corrupt registry after preserving raw metadata | Unsupported schema preserved, automatic backup before restore |
| GUI completeness | Manual records dialog, provenance/time/expiry, backup management and confirmation | 27 frontend tests and browser fixture passed, including separate manual/official data and reviewed restore; real native acceptance separately |
| Desktop distribution | Windows amd64 setup EXE, Linux amd64 portable archive, macOS amd64 + arm64 DMGs; separate OTA archives | Four native CI builds and installer/DMG checks configured; publishing requires successful release-commit CI |
| Download and update trust | User-selected unsigned EXE/DMG distribution, SHA-256 checksums and mandatory Ed25519-signed OTA manifest | OS certificate signing/notarization is optional and not implemented or claimed; release requires artifact/signature verification |
| Existing launch/handoff/OTA | Regression review and executable-level checks; update docs accurately | Core/desktop tests, race/vulnerability checks, signed public release download |
| Real-platform acceptance | Two accounts, native terminals, actual upgrade/rollback and OS identity isolation | Requires owner login/devices; never replaced by fixtures |

Future Gateway, automatic account rotation, shared transcripts, cloud sync and a privileged background service are outside the accepted design. GUI updates operate while the app is open. No new schema is invented solely to claim a migration; a real schema change must include a reversible migration compatible with OTA rollback.

## Task 1: Manual records

Files: internal/usage/manual.go and tests; internal/cli/usage.go; internal/desktop/manual.go and service view; desktop bindings/frontend.

1. Write and run failing persistence/validation tests: official data unchanged, finite 0..100 percentages, future reset/limit times, registered stable ID, strict bounded JSON and symlink containment.
2. Implement ManualRecord {observedAt, fiveHour, sevenDay, limitedUntil, source:"manual"} in a separate local file; source and observedAt are set by the server. No credentials or arbitrary free-text fields.
3. Add CLI usage show/record/clear and desktop RecordManualUsage/ClearManualUsage. Manual values never masquerade as official refreshes or trigger an automatic switch.
4. Add GUI form and explicit source/time/expired states; meaningful frontend tests, then core regression tests.

## Task 2: Local configuration recovery

Files: internal/state/backup.go and tests; internal/cli/backup.go; internal/desktop/backup.go; desktop bindings/frontend.

1. Write failing tests for account-ID/project/default round trip, active sessions, corruption, root mismatch, invalid names and unknown backup versions.
2. Add private local snapshots of tool-owned registry only. No official profile traversal. Backup envelope binds to canonical local data root; public portable export stays label-only.
3. Preview and restore a selected local backup only with explicit review; lock all affected account IDs, preserve current registry as pre-restore backup, atomically replace registry. Corrupt current bytes are preserved for recovery without exposing contents.
4. Expose list/create/preview/restore through CLI and GUI. Keep the current v1 schema and document future migration requirements honestly.

## Task 3: Release completeness

Files: .github/workflows/ci.yml, scripts/build-desktop.ps1, installer/DMG packaging and smoke scripts, existing OTA release-signing tools/docs, docs/compatibility.md.

1. Verify currently available native runner labels from GitHub's official documentation; add Intel macOS without removing ARM.
2. Build the requested unsigned Windows setup EXE and both macOS DMGs; publish checksums and the existing mandatory Ed25519 signature for OTA archives. No OS certificate is required for this accepted distribution mode.
3. Document optional Authenticode/Developer ID signing and notarization honestly: no certificate workflow is implemented or tested in this scope. Signed macOS bundles would also require whole-bundle OTA replacement before that combination can be supported; neither is a gate for the selected unsigned beta.
4. Run all applicable tests/builds, inspect artifacts, independent review, push complete source and publish a tested next preview/RC with a release-specific acceptance matrix. Do not relabel alpha as stable without real acceptance evidence.

## Execution

Use the isolated worktree .scratch/worktrees/complete-modules. Root owns manual records, CLI/service integration and release engineering; subagent owns configuration recovery and then frontend integration, with independent reciprocal review. Do not touch real credentials or perform real authentication automatically.

## Current verification

Local frontend evidence: 27 tests passed, TypeScript/Vite production build passed,
and the browser fixture passed manual-record provenance, backup creation/preview/
reviewed restore, login onboarding, handoff and update controls. The fixture is a
test-only native bridge replacement, not real Claude authentication or a native
terminal acceptance test. Core/service temporary-data tests cover registry recovery
and preservation of profile data. The compiled CLI's isolated manual-record,
backup, rename, reviewed restore and clear loop also passed without login.
Publication requires successful release-commit CI, four native desktop artifacts,
installer/DMG checks and release signature verification. Exact CI and checksum
evidence belongs to the [versioned release](https://github.com/harveyxiacn/cc-router/releases/tag/v0.2.0-beta.1),
not an older alpha run.
