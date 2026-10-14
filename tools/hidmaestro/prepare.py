"""Fetch/verify the pinned upstream SDK, or record a published helper manifest."""
from pathlib import Path
import hashlib
import json
import shutil
import sys
import urllib.request
import zipfile

root = Path(__file__).resolve().parents[2]
sdk = root / "build/hidmaestro-sdk"
archive = root / "build/HIDMaestro-v1.11.0.zip"
url = "https://github.com/hifihedgehog/HIDMaestro/releases/download/v1.11.0/HIDMaestro-v1.11.0.zip"
digest = "51c5a957feb7573cf60ee8f30f791dd013053208eaf23d013e28b438303fbdf2"
files = {
    "HIDMaestro.Core.dll": "HIDMaestro.Core.dll",
    "HIDMaestroTest\\Microsoft.Windows.SDK.NET.dll": "Microsoft.Windows.SDK.NET.dll",
    "HIDMaestroTest\\WinRT.Runtime.dll": "WinRT.Runtime.dll",
    "LICENSE": "LICENSE-HIDMaestro.txt",
    "THIRD-PARTY-NOTICES.txt": "THIRD-PARTY-NOTICES-HIDMaestro.txt",
}
hidhide = root / "build/HidHide_1.5.230_x64.exe"
hidhide_url = "https://github.com/nefarius/HidHide/releases/download/v1.5.230.0/HidHide_1.5.230_x64.exe"
hidhide_digest = "f4bbbcb82e6258641b887c74bc81c4c5f66e4aa811808dfc304347687b7605f6"

def sha(path):
    with path.open("rb") as file:
        return hashlib.file_digest(file, "sha256").hexdigest()

if len(sys.argv) == 1:
    archive.parent.mkdir(parents=True, exist_ok=True)
    if not archive.exists():
        temp = archive.with_suffix(".tmp")
        try:
            with urllib.request.urlopen(url, timeout=120) as response, temp.open("wb") as output:
                shutil.copyfileobj(response, output)
            if sha(temp) != digest:
                raise RuntimeError("HIDMaestro archive checksum mismatch")
            temp.replace(archive)
        finally:
            temp.unlink(missing_ok=True)
    if sha(archive) != digest:
        raise RuntimeError("HIDMaestro archive checksum mismatch")
    sdk.mkdir(parents=True, exist_ok=True)
    with zipfile.ZipFile(archive) as package:
        for source, destination in files.items():
            (sdk / destination).write_bytes(package.read(source))
    if not hidhide.exists():
        temp = hidhide.with_suffix(".tmp")
        try:
            with urllib.request.urlopen(hidhide_url, timeout=120) as response, temp.open("wb") as output:
                shutil.copyfileobj(response, output)
            if sha(temp) != hidhide_digest:
                raise RuntimeError("HidHide installer checksum mismatch")
            temp.replace(hidhide)
        finally:
            temp.unlink(missing_ok=True)
    if sha(hidhide) != hidhide_digest:
        raise RuntimeError("HidHide installer checksum mismatch")
    print("Prepared verified HIDMaestro v1.11.0 SDK")
    print("Prepared verified HidHide 1.5.230 installer")
else:
    output = root / "build/bin/assets/hidmaestro"
    for name in ["LICENSE-HIDMaestro.txt", "THIRD-PARTY-NOTICES-HIDMaestro.txt"]:
        shutil.copyfile(sdk / name, output / name)
    if sha(hidhide) != hidhide_digest:
        raise RuntimeError("HidHide installer checksum mismatch")
    shutil.copyfile(hidhide, output / hidhide.name)
    shutil.copyfile(root / "tools/hidmaestro/LICENSE-HidHide.txt", output / "LICENSE-HidHide.txt")
    manifest = {path.name: sha(path) for path in sorted(output.iterdir()) if path.is_file()}
    if "StageBox.HIDMaestro.exe" not in manifest:
        raise RuntimeError("Portable helper has not been published")
    (root / "internal/platform/controller/hidmaestro_manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
    print("Recorded portable helper integrity manifest:", len(manifest), "files")
