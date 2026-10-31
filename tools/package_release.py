"""Validate a paired Windows bundle and create its release ZIP and SHA-256 file."""

import argparse
import hashlib
import json
from pathlib import Path
import re
import zipfile


def sha256(path: Path) -> str:
    with path.open("rb") as source:
        return hashlib.file_digest(source, "sha256").hexdigest()


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", help="default: the appVersion literal in main.go")
    parser.add_argument("--source-commit", required=True)
    parser.add_argument("--bundle", type=Path, default=Path("release"))
    parser.add_argument("--output", type=Path, default=Path("build/release-assets"))
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[1]
    if args.version is None:
        match = re.search(r'^var appVersion\s*=\s*"([^"\n]+)"', (root / "main.go").read_text(), re.MULTILINE)
        if not match:
            parser.error("cannot read the source version; supply --version explicitly")
        args.version = match.group(1)
    if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._-]{0,79}", args.version):
        parser.error("version must be a file-safe tag such as v0.43 or v0.43-beta.1")
    if not re.fullmatch(r"[0-9a-f]{40}", args.source_commit):
        parser.error("source-commit must be a full Git commit SHA")

    bundle = args.bundle.resolve()
    if args.output.resolve().is_relative_to(bundle):
        parser.error("output must be outside the bundle directory")
    required = (
        "stagebox.exe", "assets/hidmaestro/StageBox.HIDMaestro.exe",
        "install-stagebox.ps1", "install-hidhide.ps1", "svc.vbs",
        "systemsvcs.ps1", "elevated-PS.cmd", "config.example.json",
        "START-HERE.txt", "LICENSE", "THIRD_PARTY_NOTICES.md", "docs/ARTWORK.md",
        "licenses/go/github.com/wailsapp/wails/v2/LICENSE",
    )
    for name in required:
        if not (bundle / name).is_file():
            raise SystemExit(f"Incomplete release bundle: missing {name}")
    for path in bundle.rglob("*"):
        if path.is_symlink():
            raise SystemExit(f"Release bundle must not contain a symlink: {path}")
        if path.name in ("config.json", "controllers.json", "hidhide-session.json") or path.name.endswith((".log", ".log.previous")):
            raise SystemExit(f"Local runtime data must not be released: {path}")

    manifest = json.loads((root / "internal/platform/controller/hidmaestro_manifest.json").read_text())
    if not isinstance(manifest, dict) or "StageBox.HIDMaestro.exe" not in manifest:
        raise SystemExit("Missing published helper manifest; run make release first")
    for name, expected in manifest.items():
        path = bundle / "assets" / "hidmaestro" / name
        if Path(name).name != name or not path.is_file() or sha256(path) != expected:
            raise SystemExit(f"Helper integrity mismatch: {name}; rebuild the paired bundle")

    metadata = {
        "version": args.version,
        "sourceCommit": args.source_commit,
        "platform": "windows/amd64",
        "source": f"https://github.com/aetoledano/stagebox/tree/{args.source_commit}",
    }
    (bundle / "BUILD-INFO.json").write_text(json.dumps(metadata, indent=2) + "\n", encoding="utf-8")
    args.output.mkdir(parents=True, exist_ok=True)
    archive = args.output / f"stagebox-{args.version}-windows-x64.zip"
    with zipfile.ZipFile(archive, "w", compression=zipfile.ZIP_DEFLATED, compresslevel=9) as package:
        for path in sorted(bundle.rglob("*")):
            if path.is_file():
                package.write(path, path.relative_to(bundle).as_posix())
    checksum = archive.with_suffix(archive.suffix + ".sha256")
    checksum.write_text(f"{sha256(archive)}  {archive.name}\n", encoding="utf-8")
    print(f"Created {archive} ({archive.stat().st_size / 1024 / 1024:.1f} MiB)")
    print(f"Created {checksum}")


if __name__ == "__main__":
    main()
