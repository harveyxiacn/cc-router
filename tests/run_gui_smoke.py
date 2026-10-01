"""Start Vite directly so the process is also cleaned up reliably on Windows."""
import os
import shutil
import socket
import subprocess
import sys
import time
import urllib.request
from pathlib import Path

root = Path(__file__).resolve().parent.parent
node = shutil.which("node")
if not node:
    raise RuntimeError("Node.js is required")
with socket.socket() as check:
    check.bind(("127.0.0.1", 5173))
(root / ".scratch").mkdir(exist_ok=True)
with (root / ".scratch" / "gui-test-server.log").open("w", encoding="utf-8") as log:
    process = subprocess.Popen(
        [node, "node_modules/vite/bin/vite.js", "--host", "127.0.0.1", "--port", "5173", "--strictPort"],
        cwd=root / "desktop" / "frontend", stdout=log, stderr=subprocess.STDOUT,
        creationflags=subprocess.CREATE_NO_WINDOW if os.name == "nt" else 0,
    )
    try:
        for attempt in range(100):
            if process.poll() is not None:
                raise RuntimeError("Vite exited; inspect .scratch/gui-test-server.log")
            try:
                with urllib.request.urlopen("http://127.0.0.1:5173", timeout=1):
                    break
            except OSError:
                time.sleep(0.1)
        else:
            raise RuntimeError("Vite did not start within 10 seconds")
        subprocess.run([sys.executable, str(root / "tests" / "gui_smoke.py")], cwd=root, check=True)
    finally:
        process.terminate()
        try:
            process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait(timeout=5)
