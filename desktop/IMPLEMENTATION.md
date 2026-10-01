# Desktop implementation plan

Goal: provide a Simplified Chinese desktop workspace for the existing account,
project, handoff, diagnostic, and official usage services.

Architecture: Wails v2.16 hosts React and TypeScript. A thin Go App forwards to
the root internal/desktop service. No frontend mock backend or example accounts
are provided. Only GetSnapshot is polled, at five-second intervals.

1. Verify quota unknown, stale, expiry and threshold semantics with failing unit tests.
2. Implement the typed Wails binding and quota view models.
3. Build the account workspace, project picker and binding, handoff editor with
   digest conflict handling, and explicit identity/diagnostic actions.
4. Bind Go methods, include built frontend assets, and preserve initialization errors.
5. Test form accessibility, validation, unknown quota and handoff review controls.
6. Build frontend and Windows executable; document native Linux/macOS build prerequisites.

Visual direction: ink navigation, warm paper workspace, generous editorial type,
fine ruled cards, botanical green actions, and restrained amber usage warnings.
