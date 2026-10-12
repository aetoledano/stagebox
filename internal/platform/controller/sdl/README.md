# Bundled SDL3 controller runtime

StageBox bundles the unmodified official SDL 3.4.18 Windows x64 runtime and its
zlib license. Only the gamepad/joystick/events subsystems are used; Wails owns
the window, rendering and audio UI.

- Release: https://github.com/libsdl-org/SDL/releases/tag/release-3.4.18
- Archive: `SDL3-3.4.18-win32-x64.zip`
- Archive SHA-256: `75c2c0fc74e7d1206aaedc22893a35887332f4f85f5c8b74fddc10d177985f2c`
- `windows_amd64/SDL3.dll` SHA-256: `2b4b128d8d510cc2d442693cf848c050d700d43dc667afe9e5fd82b0570d667e`
- License: `windows_amd64/LICENSE.txt`, copied verbatim from the release archive.

The archive digest was checked against the official GitHub release metadata.
No downloaded executable was run in the development environment. The DLL's
static imports are Windows system DLLs, so no extra CRT DLL deployment is needed.

The Windows amd64 Go build embeds both files and extracts versioned copies into
`cache/assets` at runtime. Load only that absolute DLL path. Missing/unwritable
runtime files or SDL initialization failure select the legacy input backend.
Updating SDL requires replacing the DLL/license, checking official digests and
PE imports, and updating the pinned filenames in `sdl_runtime_windows_amd64.go`.

## Controller databases and overrides

SDL's built-in list is compiled into its DLL. StageBox does not embed or extract
another mapping list. The full current upstream database contains the Ultimate
`2dc8:3012` mapping that was previously shipped separately; the former subset
and cache extraction code have been removed. Existing generated
`cache/assets/stagebox-gamecontrollerdb-v1.txt` and its companion license are
unused and can be deleted; they will not be recreated.

Runtime precedence, all external paths relative to `stagebox.exe`:

1. SDL built-ins/environment mappings.
2. `assets/gamecontrollerdb.txt`: complete downloaded/cached upstream base.
3. `gamecontrollerdb.txt`: legacy executable-adjacent manual overrides.
4. `assets/gamecontrollerdb-overrides.txt`: primary manual overrides, loaded last.

Only the upstream database license is embedded from `gamecontrollerdb-LICENSE.txt`
and copied to the assets folder. It was preserved verbatim from upstream
revision `c1d5289a1f713b30a2c121e9fe6529d39360be0f`.

StageBox prepares an empty commented overrides template only when missing.
Existing overrides are never overwritten. `downloadControllerDatabase` defaults
to true: every Windows startup launches a separate Go goroutine to fetch the
upstream raw master database URL, even when a cached database already exists.
Setting the option false disables the request but still prepares/loads/watches
local mappings. The SDL owner and app startup never await network activity.

The request has a 30-second timeout, an 8MiB limit and startup/stop context
cancellation. Validate HTTP status, exact declared length, UTF-8/row structure
(including SDL's special `xinput` GUID) and Windows entries. Only then sync a
same-directory temporary file and atomically promote it with Windows
`MoveFileEx`/`REPLACE_EXISTING`. Identical content skips rewriting and reload.
Failures keep the existing base. Before the first changed-base replacement,
retain its original bytes in `assets/gamecontrollerdb-before-refresh.txt` only
if that backup is absent, allowing migration of previous manual base edits.
The embedded upstream license is copied beside the base only when missing.
Only the managed base is replaced; no recurring request during a session.

The base and both overrides sources are checked every second; two identical
valid snapshots trigger live reload, normally one to two seconds after a save.
SDL's loader is additive, so reinitialize only private SDL input on its locked
owner thread to undo deleted rows/files as well as apply changed ones. A mapping
revision resets held mapped keys, prior buttons, repeat/volume/kill state and
the neutral gate even for reused instance IDs. Controllers are briefly rescanned.
Invalid/unreadable changes retain the last loaded mappings. A failed SDL reload
restores the previous snapshot or uses native fallback if restoration fails.
Read validated snapshots via `SDL_IOFromConstMem`, keeping Go bytes alive through
the synchronous SDL load/close to avoid file-read races.

Use UTF-8 `platform:Windows` entries for Windows overrides. Bluetooth/USB,
firmware and mode can change GUIDs/layouts. Do not copy Batocera/Linux axis
layouts by changing only the platform. Connection logs include names, VID/PID,
mapped status and complete selected mappings. The full downloaded database can
replace matching existing SDL profiles, while manual overrides remain final.
