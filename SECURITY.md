# Security boundaries

CC Router is an alpha local process launcher. It does not read `.credentials.json`, extract Keychain credentials, implement OAuth, proxy requests, or share Claude history between accounts.

## Controls

- Stable random account IDs; profile paths do not depend on user-provided labels.
- Direct subprocess arguments and inherited terminal streams; no shell command construction for launching Claude.
- Launch-time authentication/configuration conflict checks. Findings contain source and key names, not values.
- Strict bounded metadata JSON, duplicate/unknown key rejection, corruption preservation, cross-process locks and replacement writes.
- New private directories use 0700 on Unix and an owner-only inheritable ACL on Windows. Existing official profile ACLs are preserved.
- Exports explicitly allow only account names, labels and default account name. No profile paths, project bindings or official files are exported.
- Handoff collects filtered Git status paths only and never executes its contents. Usage consumes documented statusline stdin fields and ignores transcript paths.
- GitHub Actions have read-only repository permissions and pinned action commits. The CLI/core uses the Go standard library; the desktop has separately locked Wails/frontend dependencies.
- The desktop uses local Wails bindings and renders handoff text as an editable plain-text value. It has no credential entry form. Native terminals launch the companion CLI with the explicit data root.
- Desktop updates require an embedded Ed25519 public key, an authenticated manifest, exact SHA-256/size checks and bounded, link-free extraction. The helper re-verifies the signed archive, requires an exclusive installation lease, backs up replaced files and rolls back an unsuccessful startup. Release signing keys are kept outside Git; Windows protects the local maintainer key with current-user DPAPI.

## Limits

This is not an isolation sandbox. Code and other processes running as the same OS user can read or change profiles, settings and handoff files. Local filesystem races against that user are not fully prevented by metadata locks. Use a private local filesystem; network filesystems, shared/cloud-synced roots and adversarial multi-user deployment are unsupported.

Existing custom data roots must already have appropriate ownership and permissions. CC Router does not recursively rewrite official profile permissions or delete credential files. Metadata replacement protects consistency but is not a backup or a guarantee against every filesystem/power failure.

The launcher trusts the installed official executable (or explicit `CCR_CLAUDE_BIN`) and its dependencies. The executable can load project hooks/MCP servers according to official trust controls. Settings can change during a running session; a successful static check is not a perpetual identity/billing guarantee. New environment variables, managed policy mechanisms, unknown CLI schemas and platform changes require maintenance.

Usage percentages are last-reported snapshots; thresholds do not guarantee a remaining token reserve. The statusline may omit windows or model-specific limits. A missing value is unknown. No automatic quota rotation, request retry or task replay occurs.

Usage setup edits only the selected profile's `statusLine` setting after an explicit user action, preserves unrelated settings, and refuses an existing statusline. The command path must stay stable. Cache files retain only usage/reset/observation fields, never raw statusline input. GUI edits to handoff notes use digest checks to detect a previously saved external edit; this is not a filesystem-wide transaction against other editors.

Desktop binaries require native OS WebView components. Windows/Linux terminal launches use argument arrays/native process APIs; macOS Terminal.app uses a private, narrowly quoted `.command` bridge. These adapters still need interactive real-device acceptance. Development servers are separate from production desktop distribution.

Session locks are advisory and apply only to this launcher and the selected canonical directory. They cannot detect every external Claude process or overlapping nested project. Background/daemon tasks and real terminal shutdown need explicit platform verification before a stable release.

The updater trusts the embedded release key and the already installed companion CLI. It is not a full TUF deployment: there is one signing key, no threshold/offline root rotation protocol, and no independent timestamp service. Expired manifests and non-advancing automatic versions are rejected; a compromised signing key remains a release compromise. Authenticode/notarization are not supplied in this alpha. Files are synced and journaled, but recovery is not a guarantee against arbitrary power loss or filesystem corruption. See [updates](docs/updates.md) for portable-installation limits, health checks and rollback behavior.

Report reproducible security issues without tokens, real conversation history or identifying account data. Use GitHub private vulnerability reporting when enabled; do not publish credentials in an issue. See [compatibility](docs/compatibility.md) for tests that still need real devices/accounts.
