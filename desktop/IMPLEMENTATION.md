# Desktop implementation and verification

Goal: provide a Simplified Chinese desktop workspace for the existing account,
project, handoff, diagnostic, and official usage services.

Architecture: Wails v2.16 hosts React and TypeScript. A thin Go App forwards to
the root internal/desktop service. The shipped frontend requires the native Wails
bridge and has no example accounts. GetSnapshot and GetUpdateStatus are polled at
five-second intervals. Identity/diagnostics are explicit actions; they are never polled.

Implemented account create/rename/remove/default, folder selection and project
binding, usage display and statusline installation, identity/diagnostics,
digest-aware handoff editing and reviewed manual switching, and native metadata
import/export. Creation offers a default-checked official login action; login can
run before choosing a project. A failed terminal launch preserves the new account.

The updates area forwards checking, download/staging, automatic-update preference,
manual restart, rollback and fixed official release navigation to the Go service.
The UI reports readiness only after committing a successful local snapshot, and
reports idle only when drafts/dialogs/operations are absent and interaction has
stopped for 30 seconds. Explicit manual restart skips the delay. Restarting makes
the workspace inert. Signature/download/transaction logic remains in the core;
see [update design and boundaries](../docs/updates.md).

Verification on Windows: 18 frontend unit/component tests, TypeScript/Vite build,
desktop Go tests/vet, official-registry npm audit (zero findings), and the real
React UI browser smoke through an explicit test-only native bridge fixture.
The browser smoke checks honest quotas, creation/login opt-in, binding, reviewed
switching, preserved conflicting drafts and update controls. It accesses no real
Claude account. Native window startup and platform builds are separate acceptance
evidence; full release-to-release OTA and official browser login still require
target-machine acceptance. Linux/macOS core update tests have also been compiled;
native desktop builds run in the repository CI.

Visual direction: ink navigation, warm paper workspace, generous editorial type,
fine ruled cards, botanical green actions, and restrained amber usage warnings.
