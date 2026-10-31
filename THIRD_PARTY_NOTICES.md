# Third-party notices

StageBox's original code is licensed under GPL-3.0-only; that designation does
not replace the licenses of the components and artwork listed here.

| Component | License and retained notice | Location / provenance |
| --- | --- | --- |
| Go dependencies | Individual upstream licenses retained in each vendored module | `vendor/`, `vendor/modules.txt`, `go.mod` |
| Wails frontend runtime | MIT; upstream notice in `vendor/github.com/wailsapp/wails/v2/LICENSE` | `frontend/wailsjs/runtime/` |
| SDL 3.4.18 Windows x64 runtime | zlib; `internal/platform/controller/sdl/windows_amd64/LICENSE.txt` | [Pinned source and checksums](internal/platform/controller/sdl/README.md) |
| SDL_GameControllerDB | zlib; `internal/platform/controller/sdl/gamecontrollerdb-LICENSE.txt` | Downloaded at runtime; license embedded and extracted beside it |
| Steam vector icon | CC0; [Simple Icons license](https://github.com/simple-icons/simple-icons/blob/develop/LICENSE.md) | `frontend/src/assets/images/steam-ico.svg`; [upstream icon](https://github.com/simple-icons/simple-icons/blob/develop/icons/steam.svg) |
| HIDMaestro v1.11.0 SDK | MIT plus upstream third-party notices, copied by the helper build | [Build/provenance](tools/hidmaestro/README.md); downloaded to ignored `build/` |
| HidHide 1.5.230 installer | MIT; `tools/hidmaestro/LICENSE-HidHide.txt` | Unmodified pinned installer downloaded by `tools/hidmaestro/prepare.py` |
| Self-contained .NET runtime | Upstream licenses and notices retained in helper publish output | `build/bin/assets/hidmaestro/` after publishing |
| Frontend build dependencies | Individual package licenses installed by npm | `frontend/package.json` and `frontend/package-lock.json` |

Steam, Xbox and RetroBat names and logos identify their respective products;
StageBox does not claim ownership or affiliation. The owner reports that most
supplied artwork was AI generated, with some icons downloaded from free sites.
The retained assets and known generation records are listed in
[docs/ARTWORK.md](docs/ARTWORK.md); exact source/license records for some active
icons remain incomplete. The RetroBat SVG is a project trace of a supplied logo
reference. Do not assume the code license grants rights to third-party artwork.
Unused artwork and the unused Nunito font have been removed from the current
tree; those files remain in preserved Git history.

Distribute the release with its license/notice files and corresponding source.
The `make release` target includes the repository license, this notice, the
artwork inventory, vendored Go notices and the matching helper notices. Retain the notices carried by
vendored dependencies when distributing source.
