"""Enable NoBackgroundThrottling for MyGo's native macOS surface."""
import hashlib
import json
import pathlib
import shutil
import subprocess

root = pathlib.Path(__file__).resolve().parent.parent
metadata = json.loads(subprocess.check_output(
    ["go", "mod", "download", "-json", "github.com/egoist/mygo@v0.7.4"], text=True))
module = pathlib.Path(metadata["Dir"])
source = module / "internal/darwin/surface.go"
if hashlib.sha256(source.read_bytes()).hexdigest() != "88c3d595640badd5097aca94a96d935453230d512d539df0008d6b618b30c3cc":
    raise SystemExit("MyGo source changed; review the native background refresh patch")
text = source.read_text()
old = """func (s *surface) RequestFrame() {
\tif s.w.closed {
\t\treturn
\t}
"""
new = old + """\t// Keep controller status and accessibility current even when the OS
\t// marks a management window as occluded or stops its display link.
\tif s.w.opts.NoBackgroundThrottling {
\t\ts.drawSoon()
\t\treturn
\t}
"""
old_occluded = "func (s *surface) Occluded() bool { return !s.visible() }"
new_occluded = "func (s *surface) Occluded() bool { return !s.w.opts.NoBackgroundThrottling && !s.visible() }"
if text.count(old) != 1 or text.count(old_occluded) != 1:
    raise SystemExit("MyGo refresh implementation changed; review the compatibility patch")
dest = root / ".cache/mygo-compat"
dest.mkdir(parents=True, exist_ok=True)
copy = dest / "mygo"
if not (copy / "go.mod").exists():
    shutil.copytree(module, copy)
    for directory in [copy, *[p for p in copy.rglob("*") if p.is_dir()]]:
        directory.chmod(0o755)
patched = copy / "internal/darwin/surface.go"
patched.chmod(0o644)
patched.write_text(text.replace(old, new).replace(old_occluded, new_occluded))
modfile = dest / "nativejet.mod"
modfile.write_text((root / "go.mod").read_text() + "\nreplace github.com/egoist/mygo => " + json.dumps(str(copy)) + "\n")
shutil.copyfile(root / "go.sum", dest / "nativejet.sum")
print(modfile)
