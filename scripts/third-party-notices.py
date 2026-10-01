"""Regenerate binary distribution notices after Go/npm dependencies are installed."""
import json
import subprocess
import sys
from pathlib import Path

root = Path(__file__).resolve().parent.parent
go = sys.argv[1] if len(sys.argv) > 1 else "go"
desktop = root / "desktop"

def command(*args):
    return subprocess.check_output([go, *args], cwd=desktop, text=True, encoding="utf-8")

def objects(text):
    decoder = json.JSONDecoder()
    while text.strip():
        value, end = decoder.raw_decode(text.lstrip())
        yield value
        text = text.lstrip()[end:]

sections = ["CC Router third-party notices\n\nGenerated from the desktop Go dependency graph and bundled frontend packages.\nSome dependencies are used only on particular platforms or during builds.\nOperating-system WebViews are not redistributed in these archives.\n"]
missing = []

def collect(name, version, directory):
    directory = Path(directory)
    files = sorted(path for path in directory.iterdir() if path.is_file() and
                   (path.name.lower().startswith("license") or path.name.lower() in ("copying", "notice")))
    if not files:
        missing.append(name)
        return
    sections.append("\n" + "=" * 72 + f"\n{name} {version}\n" + "=" * 72 + "\n")
    for path in files:
        sections.append(f"\n[{path.name}]\n" + path.read_text(encoding="utf-8").rstrip() + "\n")

for module in objects(command("list", "-m", "-json", "all")):
    if module.get("Main") or module["Path"].startswith("github.com/harveyxiacn/cc-router"):
        continue
    directory = module.get("Dir")
    if not directory:
        directory = json.loads(command("mod", "download", "-json", module["Path"] + "@" + module["Version"]))["Dir"]
    collect(module["Path"], module["Version"], directory)

for package in ("react", "react-dom", "scheduler", "lucide-react", "vite"):
    directory = desktop / "frontend" / "node_modules" / package
    info = json.loads((directory / "package.json").read_text(encoding="utf-8"))
    collect(info["name"], info["version"], directory)

go_root = command("env", "GOROOT").strip()
collect("Go runtime and standard library", command("env", "GOVERSION").strip(), go_root)
if missing:
    raise RuntimeError("Missing license files: " + ", ".join(missing))
(root / "THIRD_PARTY_NOTICES.txt").write_text("\n".join(line.rstrip() for line in "".join(sections).splitlines()) + "\n", encoding="utf-8", newline="\n")
print("Wrote THIRD_PARTY_NOTICES.txt from installed, versioned dependencies")
