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
- GitHub Actions have read-only repository permissions and pinned action commits; runtime has no third-party Go modules.

## Limits

This is not an isolation sandbox. Code and other processes running as the same OS user can read or change profiles, settings and handoff files. Local filesystem races against that user are not fully prevented by metadata locks. Use a private local filesystem; network filesystems, shared/cloud-synced roots and adversarial multi-user deployment are unsupported.

Existing custom data roots must already have appropriate ownership and permissions. CC Router does not recursively rewrite official profile permissions or delete credential files. Metadata replacement protects consistency but is not a backup or a guarantee against every filesystem/power failure.

The launcher trusts the installed official executable (or explicit `CCR_CLAUDE_BIN`) and its dependencies. The executable can load project hooks/MCP servers according to official trust controls. Settings can change during a running session; a successful static check is not a perpetual identity/billing guarantee. New environment variables, managed policy mechanisms, unknown CLI schemas and platform changes require maintenance.

Usage percentages are last-reported snapshots; thresholds do not guarantee a remaining token reserve. The statusline may omit windows or model-specific limits. A missing value is unknown. No automatic quota rotation, request retry or task replay occurs.

Session locks are advisory and apply only to this launcher and the selected canonical directory. They cannot detect every external Claude process or overlapping nested project. Background/daemon tasks and real terminal shutdown need explicit platform verification before a stable release.

Report reproducible security issues without tokens, real conversation history or identifying account data. Use GitHub private vulnerability reporting when enabled; do not publish credentials in an issue. See [compatibility](docs/compatibility.md) for tests that still need real devices/accounts.
