# Desktop interaction checks

The frontend smoke test injects an explicit native-bridge fixture into the real
React app. It does not add a mock backend to the application or access accounts.
After `npm ci` in `desktop/frontend`, install Python Playwright and Chromium:

```sh
python -m pip install playwright==1.57.0
python -m playwright install chromium
python tests/run_gui_smoke.py
```

The runner starts Vite directly on localhost:5173 and terminates its own process
on exit. Screenshots and logs are ignored under `.scratch/`. Tests cover quota
unknown/95% states, account creation, project binding, explicit handoff review,
manual switching, and preservation of a draft after a save conflict.

The production Wails WebView2 loader intentionally clears browser debugging
environment variables. Browser fixture tests do not prove native dialog or
terminal behavior. Native acceptance must also verify those on each OS, using
disposable profiles and without including credentials in evidence.
