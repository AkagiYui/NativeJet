"""Regenerate the checked-in icons from SVG (requires librsvg)."""

from pathlib import Path
import struct
import subprocess


root = Path(__file__).resolve().parent.parent
macos = root / "packaging/macos"
artwork = macos / "artwork"


def render(name, *options):
    return subprocess.check_output(["rsvg-convert", *options, artwork / name])


def chunk(kind, data):
    return struct.pack(">4sI", kind, len(data) + 8) + data


# ICNS uses distinct entries for normal and Retina versions at the same size.
representations = (
    (b"icp4", 16), (b"ic11", 32), (b"icp5", 32), (b"ic12", 64),
    (b"ic07", 128), (b"ic13", 256), (b"ic08", 256), (b"ic14", 512),
    (b"ic09", 512), (b"ic10", 1024),
)
pngs = {size: render("NativeJet.svg", "-w", str(size), "-h", str(size))
        for size in sorted({size for _, size in representations})}
(macos / "NativeJet.icns").write_bytes(chunk(b"icns", b"".join(
    chunk(kind, pngs[size]) for kind, size in representations
)))
(root / "internal/app/assets/MenuBarTemplate.png").write_bytes(render("MenuBarTemplate.svg"))
