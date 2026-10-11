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

## Controller mapping additions

SDL's full built-in database is compiled into the DLL. `gamecontrollerdb.txt`
here is a StageBox-curated supplement, currently the missing Windows 8BitDo
Ultimate VID `2dc8` / PID `3012` profile. The entry is copied unchanged from
the community database at revision
`c1d5289a1f713b30a2c121e9fe6529d39360be0f` (2026-10-02):
https://github.com/mdqinc/SDL_GameControllerDB/blob/c1d5289a1f713b30a2c121e9fe6529d39360be0f/gamecontrollerdb.txt

The subset is marked as such in the file; its original zlib license is retained
verbatim in `gamecontrollerdb-LICENSE.txt`. Both are embedded and extracted to
`cache/assets/stagebox-gamecontrollerdb-v1.txt` and its license. StageBox loads
this supplement first, then the older **user-owned** `gamecontrollerdb.txt`
beside `stagebox.exe`, then `assets/gamecontrollerdb.txt` beside that executable.
The assets file takes final precedence and both external files are watched for
content changes every second. Two identical valid snapshots trigger a reload
(normally one to two seconds after a save); edits do not require a restart.
Edit the repository supplement and rebuild to change shipped defaults; do not
edit the generated cache copy.

Optional `config.json` `downloadControllerDatabase: true` downloads the current
full community database from the upstream raw master URL when the assets file
is missing. The default is false; existing files are never overwritten. The
background request has a 30-second timeout, an 8MiB maximum and startup/stop
context cancellation. Validate HTTP status, exact declared length, UTF-8/row
structure and at least one Windows profile before writing a temporary file in
the assets directory and promoting it with Windows `MoveFileEx` without
`REPLACE_EXISTING`. A file created during the request also remains untouched.
Write the embedded upstream license alongside it only when missing. Failures
retain the local/bundled defaults. Back up/remove the assets database and
restart to request a fresh download. No recurring internet polling is added.

Live reload rebuilds only SDL input on its locked owner thread because SDL's
mapping loader is additive: removed entries must revert to built-in/supplement
defaults. A mapping revision resets the bridge's held mapped keys, previous
buttons, repeat/volume/kill state and neutral gate even if an instance ID is
reused. Controllers are briefly rescanned. Invalid/unreadable saves leave the
last valid database active; a failed SDL reload attempts to restore that
snapshot, and native fallback remains available if restoration fails. Read
bounded validated snapshots through `SDL_IOFromConstMem`, keeping the Go bytes
alive through SDL's synchronous load/close to avoid file-read races.

Only Windows mappings with a `platform:Windows` field apply on Windows.
Bluetooth, USB, firmware and controller modes may expose different GUIDs.
Do not reuse Batocera/Linux entries by merely changing the platform field:
axis/button numbering may differ. The new default applies only if SDL matches
the device's GUID; it is not a name-based or blanket 8BitDo override.

Connection logs record SDL joystick names, VID/PID and mapped status (including
unmapped devices), plus the complete mapping chosen for each opened gamepad.
Use those and the selected input backend to verify a device before changing
another profile. The working Ultimate 2C built-in profile is not replaced by
this supplement. Enabling the optional full downloaded database does load all
upstream Windows profiles and can replace existing ones.
