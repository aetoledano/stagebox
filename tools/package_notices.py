"""Copy source and dependency notices into a StageBox release directory."""

from pathlib import Path
import shutil
import sys


def main() -> None:
    root = Path(__file__).resolve().parents[1]
    destination = Path(sys.argv[1]).resolve()
    destination.mkdir(parents=True, exist_ok=True)

    for filename in ("LICENSE", "THIRD_PARTY_NOTICES.md"):
        shutil.copy2(root / filename, destination / filename)

    artwork_notice = destination / "docs" / "ARTWORK.md"
    artwork_notice.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(root / "docs/ARTWORK.md", artwork_notice)

    vendor = root / "vendor"
    for source in sorted(vendor.rglob("*")):
        if source.is_file() and source.name.lower().startswith(
            ("license", "copying", "notice", "copyright", "authors", "patents")
        ):
            target = destination / "licenses" / "go" / source.relative_to(vendor)
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(source, target)

    print(f"Copied StageBox and vendored Go notices to {destination}")


if __name__ == "__main__":
    main()
