# Project Guide

## Summary

This repository is a Wails v2 desktop launcher named `launcher`, branded in the UI as `StageBox`. It is built as a kiosk-style game launcher with a Go backend and a Vite-powered vanilla JavaScript frontend.

The app loads games from a configured game root, renders them as a fullscreen controller-friendly grid, launches selected game executables, hides/restores the launcher window around game sessions, and provides Windows kiosk helpers such as fullscreen mode, Bluetooth status/pairing, Steam session launch, display resolution setup, wallpaper hosting, shell registration, shutdown, and restart.

The project is Windows-focused at runtime, but many platform-specific tools have non-Windows stubs so parts of the code can still compile or be inspected on Linux.

## Main Layout

- `main.go`: Wails entry point. Embeds `frontend/dist`, loads `config.json`, extracts embedded runtime assets, binds `App`, and starts Wails.
- `app.go`: Wails-bound backend API used by the frontend. Owns app lifecycle, game loading/launching, Bluetooth status events, controller bridge setup, and system actions.
- `assets`: root-level embedded runtime assets that are needed by native backend code, currently `loading.jpg`.
- `internal/assets`: runtime asset helpers, including embedded asset extraction, thumbnail cache path, and Wails thumbnail HTTP handler.
- `internal/config`: JSON config shape. Currently `gamesRoot` and `debug`.
- `internal/games`: game metadata model, concurrent game directory loader, thumbnail generation/cache support, launch/kill tracking, and Steam session launching.
- `internal/jsonx`: strict JSON file loader used by config and game metadata loading.
- `internal/logx`: small logging package with file/stdout support and panic recovery.
- `internal/platform`: platform integration packages. Windows files contain Win32/PowerShell integrations; `*_stub.go` files keep non-Windows builds safe.
  - `bluetooth`: Bluetooth state, bring-up, and connected-device detection.
  - `controller`: XInput polling, launcher keyboard hook, and Wails input events.
  - `cursor`: cursor visibility helpers.
  - `display`: display resolution selection.
  - `shell`: per-user shell registration.
  - `wallpaper`: native wallpaper host.
  - `wifi`: Windows WiFi scanning and connection helpers backed by `netsh wlan`.
  - `winapi`: shared Windows DLL handles.
- `frontend/src`: vanilla JS frontend modules and CSS.
- `frontend/src/features`: frontend feature modules for Bluetooth, clock, and controller navigation.
- `frontend/wailsjs`: generated Wails bindings used by frontend imports.
- `custom_scripts`: Windows kiosk helper scripts. Keep this folder small: `install-stagebox.ps1` for elevated machine setup, `svc.vbs` for the hidden StageBox shell loop, `systemsvcs.ps1` for service/adapters priming, and `elevated-PS.cmd` for manual elevated testing.
- `vendor`: committed Go vendor tree.

## Runtime Data Flow

1. `main.go` creates a default config and overlays `config.json` with `jsonx.LoadJSONFile`.
2. `main.go` extracts embedded runtime assets, including `assets/loading.jpg`, to `cache/assets` because the native Windows wallpaper host needs a filesystem path.
3. `NewApp` stores the config and Wails binds the `App` instance.
4. On startup, `App.startup`:
   - saves the Wails context,
   - tries to register the launcher as the per-user shell on Windows,
   - tries to force the primary display to 1920x1080 or the best lower mode,
   - starts a native wallpaper host for the extracted embedded `loading.jpg`,
   - starts the XInput controller bridge and Windows keyboard bridge,
   - starts Bluetooth status polling,
   - fullscreenes the Wails window.
5. `frontend/src/main.js` creates an empty grid, initializes controller navigation, subscribes to game loading events, then calls `LoadGames`.
6. `LoadGames` loads each game through an 8-worker pool. Each worker parses `lc.json`, hashes the source image, creates or reuses a 1280px thumbnail under `cache/thumbnails`, and emits `game:loaded` when that game is ready.
7. Cards use `/game-thumbs/<game-id>-<hash>.jpg` URLs served by the Wails asset handler instead of base64 image payloads.
8. Activating a card calls `Launch(id)`. Go pauses controller navigation, emits `game:start`, hides the launcher, starts the game process, then restores fullscreen launcher state and emits `game:end` after exit.

## Game Metadata

Games are discovered by scanning immediate subdirectories of `config.gamesRoot`. Each game directory must contain `lc.json`.

Expected fields map to `internal/games.GameData`:

```json
{
  "executable": "Game.exe",
  "image": "cover.jpg",
  "name": "Game Name",
  "description": "Optional description",
  "showCursor": false,
  "delayedKey": {
    "seconds": 4,
    "key": "Enter"
  },
  "keyMap": {
    "mouse": "rightStick",
    "mouseSensitivity": 14,
    "buttons": {
      "A": ["Space", "Enter"],
      "B": ["Escape"],
      "L3": ["Shift"],
      "R3": ["E"],
      "LT": ["MouseRight"],
      "RT": ["MouseLeft"],
      "DPAD_UP": ["Z", "ArrowUp"]
    },
    "leftStick": {
      "up": ["W"],
      "down": ["S"],
      "left": ["A"],
      "right": ["D"]
    }
  }
}
```

Notes:

- `id` is optional in JSON. If omitted, the loader creates a deterministic UUID from the game name, falling back to the game directory name when the name is blank. If `id` is supplied, it must be a valid UUID.
- JSON decoding is strict. Unknown keys fail because `jsonx.LoadJSONFile` uses `DisallowUnknownFields`.
- `executable`, `image`, and `startsOn` are resolved relative to the game directory.
- Images are processed into cached 1280px max-dimension JPEG thumbnails. The frontend receives `thumbnailUrl` and `imageHash`, not base64 image data.
- JPEG, PNG, GIF, WebP, and AVIF source images are supported for thumbnail generation.
- `delayedKey` is optional and intended for games that first open a keyboard-only launcher before the game itself starts. When present, the launcher waits for the game process PID, waits `seconds`, focuses that process window when possible, and sends one keyboard stroke named by `key`. If the process exits before the delay, the key is skipped. Supported key names match the controller key-map keyboard targets, such as `Enter`, `Space`, `Escape`, `Tab`, arrow keys, letters, digits, `Delete`, `F1`-`F12`, and `Numpad0`-`Numpad9`.
- `keyMap` is optional and embedded in `lc.json`. The game key-map path is considered only when the `keyMap` key is present; if it is omitted the controller bridge is fully paused during gameplay as before. `keyMap.buttons` maps discrete gamepad buttons to arrays of key or mouse targets, so one controller input can press multiple targets. `keyMap.leftStick` and `keyMap.rightStick` map stick directions (`up`, `down`, `left`, `right`) to arrays of normal key targets. `keyMap.mouse` can be `leftStick` or `rightStick` to move the OS cursor; `mouseSensitivity` controls X-axis speed from 1-40 while Y-axis speed is intentionally half as fast, with a minimum of 1. If `keyMap` is present but has no valid bindings and no valid mouse stick, launch falls back to paused controller behavior. Supported gamepad keys include `A`, `B`, `X`, `Y`, `START`, `BACK`/`SELECT`, `LB`, `RB`, `LT`/`L2`/`LEFTTRIGGER`, `RT`/`R2`/`RIGHTTRIGGER`, `L3`, `R3`, and `DPAD_UP`/`DOWN`/`LEFT`/`RIGHT`. Supported keyboard targets include letters, digits, `Space`, `Enter`, `Escape`, `Tab`, `Shift`, `Control`, `Alt`, arrow keys, `Delete`, `F1`-`F12`, and `Numpad0`-`Numpad9`. Supported mouse button targets are `MouseLeft`, `MouseRight`, and `MouseMiddle`. See `keypad-sample.json` for a working structured mapping sample.

## Frontend Notes

- The frontend uses Vite and plain JavaScript, not a component framework.
- `frontend/index.html` contains the fixed topbar and `.content` shell.
- `frontend/src/main.js` renders the game grid dynamically.
- Keyboard navigation is emitted by the Go backend through the same `pad:nav` and `pad:press` events as controller navigation. On Windows, the keyboard bridge maps arrow keys to navigation, Enter to `A`, Escape to `B`, and Space to `START`; `frontend/src/features/controller.js` should stay focused on consuming those events.
- The topbar has a dedicated Steam button that calls `App.OpenSteam`; when Steam is installed, `OpenSteam` launches Steam as a launcher session so StageBox hides, controller navigation is paused, the normal Windows cursor is restored and parked at the bottom-right corner of the primary screen for Steam games that need mouse input, and StageBox restores after `Steam.exe` exits.
- The tools menu has a `Wi-Fi Networks` item that opens an in-app WiFi overlay. The overlay calls `App.ScanWifiNetworks` and `App.ConnectWifiNetwork`, and pauses only the launcher keyboard hook via `App.SetLauncherKeyboardPaused` while password entry is active. Controller polling stays active and `frontend/src/features/wifi.js` consumes `pad:*` events for D-pad/A/B/Start overlay navigation. Pausing keyboard input also forces the normal Windows cursor visible for the dialog; closing the overlay moves the cursor back to `(0,0)`, hides it, and resumes launcher keyboard input.
- `--accent` in `frontend/src/css/base.css` is the shared active green for card focus rings and active topbar indicators. Prefer `var(--accent)` for new active/focus green states instead of hard-coded greens.
- `frontend/src/features/controller.js` listens for Wails runtime events:
  - `pad:nav` with `{ dir: "left" | "right" | "up" | "down" }`
  - `pad:press` with `"A"`, `"B"`, `"START"`, or `"KILL"`
- `frontend/src/main.js` listens for:
  - `game:loaded` with a single game payload
  - `games:loading` with `{ loading, loaded, total }`
  - `game:load-error` with `{ path, error }`
- `frontend/src/features/bluetooth.js` listens for `status:update` events and calls `GetStatuses`.
- Wails frontend bindings are imported from `/wailsjs/go/main/App.js` and `/wailsjs/runtime`.

When adding new backend methods that the frontend calls, update the Go `App` type, then regenerate Wails bindings if needed. Do not hand-edit generated binding files unless the task is specifically about generated output.

## Backend Notes

- Wails exposes exported methods on `App` to JavaScript. Keep method signatures simple and JSON-serializable.
- `App.games` is a map keyed by `uuid.UUID`; frontend receives it as an object and normalizes it in `main.js`.
- `App.LoadGames` populates `App.games` incrementally before emitting each `game:loaded` event, so launch can work while the rest of the library is still loading.
- `Launch` is asynchronous from the UI perspective. It starts a goroutine that waits for the process and restores the launcher afterward. `internal/games.LaunchGame` returns a handle with `PID`, `Done`, and `Exited` channels so launch-time helpers such as `delayedKey` can react to process startup and cancellation without consuming the exit result.
- `App.OpenSteam` uses `internal/games.LaunchSteamSession` when Steam is installed. It starts `Steam.exe` as a launcher session, hides StageBox, pauses controller navigation, and restores the launcher after that Steam process exits.
- `assets/loading.jpg` is embedded in the Go executable, then extracted to `cache/assets/loading.jpg` on startup. The native wallpaper code still receives a file path because Windows GDI+ loads the image from disk; deployments do not need to copy a separate `loading.jpg` next to `stagebox.exe` for the default background.
- `KillRunning` kills the tracked game process tree on Windows via hidden `taskkill /T /F`, with a direct process kill fallback. This matters for launchers that spawn child game processes.
- The XInput bridge and keyboard bridge emit Wails events and have pause/resume behavior so launcher navigation does not continue while a game, Steam session, or pairing flow is active. On Windows, the controller bridge prefers XInput and falls back to `winmm` joystick polling when Bluetooth/HID controllers are present but not exposed as XInput slots; the fallback converts joystick buttons, left stick axes, and POV hat into the same launcher `pad:*` events.
- During game launch, the cursor is forced visible only when `showCursor` is true. The Windows cursor helper centers the pointer and injects a tiny synthetic mouse move so mouse-less systems can still materialize the cursor. For games without `showCursor`, the launch path temporarily replaces common Windows system cursors with transparent cursors and reasserts hidden cursor state briefly while the game window settles; system cursors are restored on game exit, startup, shutdown, restart, and debug exit. The launcher WebView also forces `cursor: none` in CSS so its pointer stays hidden even if Windows has a visible cursor. Card selection styling must come from `.gp-focus` or keyboard focus, not `.card:hover`, because invisible cursor position can otherwise make a second card look selected. `keyMap.mouse` can still move the OS cursor, but it does not make the cursor visible by itself. On game exit, mapped inputs are released, the cursor is moved to `(0,0)`, and the cursor is hidden before the launcher window is restored so the grid does not reopen with a hovered card.
- The XInput Start button opens the centered tools menu directly; B closes the menu. Holding Start+Back for about 700ms emits the `KILL` command before launcher/game input modes, so it also works while a game key-map is active.
- When `Bluetooth Pair` opens `DevicePairingWizard.exe`, the launcher hides, the XInput bridge enters pairing keyboard mode, and the wizard is repeatedly foregrounded while visible. D-pad sends normal arrow keys, left stick left/right cycles dialog focus with Shift+Tab/Tab semantics, A sends Enter, and B sends Esc. Launcher navigation events are suppressed until the wizard window closes; before restoring the launcher the cursor is moved to `(0,0)` and hidden.
- Bluetooth pairing now uses the Windows pairing wizard bridge for the user-facing flow because it reaches the same shell/device-association path as Explorer. The legacy auto-pair API and abandoned WinRT helper were removed; `internal/platform/bluetooth` now provides Bluetooth state, bring-up, and connected-device detection. Keep Windows-only code behind `//go:build windows` and add or update matching stubs for other platforms.
- `App.ExitLauncher` has two modes. In debug mode the menu labels it `Debug - Exit Launcher` and it only quits the current Wails process so the Windows service loop can relaunch it without creating a duplicate instance. In normal mode the menu labels it `Exit to Windows`; the backend writes `exit-to-windows.flag`, starts `explorer.exe`, and quits. `custom_scripts/svc.vbs` consumes that flag after the launcher exits, deletes it, starts Explorer, and stops its relaunch loop for the current session.
- `App.OpenBluetoothPairing` is exposed as the first menu item, `Bluetooth Pair`. It opens `DevicePairingWizard.exe` to use the same Windows pairing path that Explorer/Settings uses.
- `custom_scripts/install-stagebox.ps1` is the preferred elevated setup entry point for a Windows kiosk. Run it as Administrator from a staging folder containing `stagebox.exe` and the helper scripts. It creates `C:\launcher`, copies runtime files (`stagebox.exe`, `svc.vbs`, `systemsvcs.ps1`, and `elevated-PS.cmd`) but not the installer itself, verifies `C:\launcher\stagebox.exe` exists before registering shell/autologon, writes `config.json` without a UTF-8 BOM with `gamesRoot` defaulting to `C:\games` and `debug: true` unless skipped, removes legacy helper scripts and the old `launcher.exe` name when possible, logs installation output to `installation.log` including account/SID/profile diagnostics, and derives a short local username such as `gamerX` from the current Windows identity for SMB share permissions, the user logon task, and AutoAdminLogon prompts. Shell registration still targets the loaded user hive through `HKCU` and explicit `HKEY_USERS\<sid>` to avoid ambiguity. It enables/starts Wi-Fi and Bluetooth-related services, removes every existing scheduled task under `\Launcher\`, registers `\Launcher\StageBox-EnsureSystem` as a SYSTEM startup task, registers `\Launcher\StageBox-EnsureUser` as a current-user logon task, and routes both tasks through `wscript.exe //B //Nologo C:\launcher\svc.vbs systemsvcs` so the PowerShell service primer runs hidden. It optionally writes the current user's shell to `svc.vbs`. AutoAdminLogon must use a nonblank password; a blank password skips that setup to avoid broken login state. Prefer extending this installer over adding new one-off setup scripts.
- `custom_scripts/systemsvcs.ps1` is the sole service/adapters primer. Scheduled tasks run it at startup/logon; it starts `WlanSvc`, Bluetooth-related services, per-user Bluetooth service, turns the Bluetooth radio on when possible, enables candidate Wi-Fi adapters, and optionally logs `netsh wlan show drivers/interfaces`.
- `App.ScanWifiNetworks` and `App.ConnectWifiNetwork` back the `Wi-Fi Networks` overlay. On Windows they use absolute hidden `netsh.exe` commands and do not try to force-start `WlanSvc` from the non-elevated launcher; instead scan/connect retry briefly when `netsh` reports `wlansvc is not running`, because Windows can bring the service back asynchronously. Scanning runs `netsh wlan show networks mode=bssid` with a plain `show networks` fallback and annotates networks as saved when Windows has either an SSID-named profile or a StageBox profile for that SSID. Saved secure networks connect without showing the password field. Password connections create temporary WPA2 profile XML using a StageBox-owned profile name (`StageBox-<ssid-hash>`) so existing policy/all-user profiles named after the SSID are not overwritten, try current-user then all-user profile scope, then fall back to connecting an existing SSID-named profile. `systemsvcs.ps1` retries Wi-Fi autoconnect at startup/logon after ensuring `WlanSvc`, first for visible `StageBox-*` profiles and then for other saved WLAN profiles; the user logon scheduled task runs least-privilege so current-user WLAN profiles are visible. Adapter priming skips Wi-Fi Direct virtual adapters and Xbox Wireless Adapter devices, and only calls `Set-PnpDevice` when that cmdlet exists. Failed connects log `netsh wlan show interfaces/drivers/profiles` diagnostics. This avoids relying on Windows shell WiFi flyouts, which may no-op in kiosk/shell-replacement mode. `WlanSvc` should still be enabled/started by `install-stagebox.ps1` and `systemsvcs.ps1`. If scans fail with `OpenService FAILED 5` or `wlansvc is not running`, rerun `install-stagebox.ps1` as Administrator on the Windows box. If scans fail with `no wireless interface detected`, inspect `svc.log` service diagnostics for missing/disabled Wi-Fi drivers or adapters.
- The pairing wizard lifecycle is guarded against reentry. After the wizard window closes, the launcher terminates a lingering `DevicePairingWizard.exe` process before restoring itself so quick reopen attempts do not wait on Windows' delayed process cleanup.
- `status:update` includes `controllerCount`, sourced from XInput slots first and from the Windows `winmm` joystick fallback when no XInput slot is active. It also includes Wi-Fi fields `wifiAvailable`, `wifiConnected`, `wifiSSID`, and `wifiState` from `netsh wlan show interfaces`. The controller indicator renders a small inline SVG controller icon plus that count so it can inherit CSS color consistently across Windows/Linux. `btConnected` is Bluetooth-controller-specific, not the generic controller count; the Bluetooth scan uses the Win32 Bluetooth device API and only counts controller-like devices with the Bluetooth `FConnected` flag set. Avoid using present-only PnP `BTHENUM`/`BTHLEDEVICE` as a connected signal because it can mark remembered or adapter-side devices connected. The Bluetooth indicator is slashed gray when Bluetooth is off/disabled/unknown, plain gray when Bluetooth is on but no Bluetooth controller is connected, and green when a Bluetooth controller is connected. The Wi-Fi indicator is slashed gray when no Wi-Fi interface is available, gray when Wi-Fi is available but disconnected, and green when connected.
- Startup logs include `version=<value>` and `pid=<value>`. `appVersion` uses incremental pre-1.0 values such as `0.01`; keep bumping `0.xx` during Bluetooth iteration and reserve `1.0` for when Bluetooth pairing is fully working. The app writes `stagebox.log`, truncated on launcher start; `svc.vbs` and `systemsvcs.ps1` write service-loop diagnostics to `svc.log`. Installer legacy cleanup removes old log names such as `launcher.log`, `stagebox-debug.log`, and `stagebox-svc.log`.

## Build And Dev Commands

From the repo root:

```sh
make
make dev
make build-linux
make build-win
make dep
```

Command details:

- `make` builds the packaged Windows amd64 Wails executable at `build/bin/stagebox.exe`.
- `make dev` runs `wails dev -tags 'webkit2_41'`.
- `make build-linux` runs a Linux Wails build with the same tag.
- `make build-win` cross-builds a Windows amd64 executable using MinGW variables from the Makefile.
- `make dep` runs `go mod tidy` and `go mod vendor`.

Frontend-only commands live in `frontend/package.json`:

```sh
npm run dev
npm run build
npm run preview
```

Wails runs `npm install`, `npm run build`, and `npm run dev` through the settings in `wails.json`.

## Development Hints

- Prefer existing patterns: Wails-bound methods on `App`, domain packages under `internal/platform`, plain JS modules under `frontend/src/features`.
- Treat `config.json` as local runtime configuration. Avoid hard-coding paths except where the Windows kiosk/bootstrap flow already requires fixed paths.
- Preserve build tags. Every new Windows-specific file should have `//go:build windows`; add a `!windows` stub when shared code calls it.
- Keep frontend state derived from backend events where possible. Existing event names are short and status-oriented.
- Be careful with UUID handling. Frontend passes IDs as strings; Wails currently maps them back into `uuid.UUID` parameters for methods such as `Launch`.
- Avoid broad changes under `vendor` unless explicitly updating vendored dependencies or local vendor patches.
- Logging should use `internal/logx` for app diagnostics. Existing direct `log.Println` calls remain, but new app-level diagnostics should usually go through `logx`.
- `jsonx.LoadJSONFile` rejects unknown JSON fields. When adding config or game metadata fields, update the Go structs first.
- `jsonx.LoadJSONFile` tolerates a UTF-8 BOM before strict JSON decoding. This is important for Windows-generated `config.json` files from older PowerShell flows.
- Do not assume Windows helper commands work on Linux. Non-Windows stubs intentionally return no-op or `"unknown"` behavior.
- Keep thumbnail filenames hash-based and include the thumbnail size/version in the filename. The source hash invalidates cache when source images change; the size suffix forces regeneration when thumbnail dimensions change.

## Common Feature Entry Points

- Add game metadata fields: update `internal/games/data.go`, adjust `internal/games/loader.go` if path resolution or derived data is needed, then update `frontend/src/main.js` rendering.
- Add thumbnail behavior: update `internal/games/thumbnails.go`, keep output URLs under `/game-thumbs/`, and avoid putting image bytes in Wails method return payloads.
- Add a topbar action: add the button in `frontend/index.html`, bind it in `frontend/src/main.js`, and expose a Go `App` method if it needs backend behavior.
- Add controller behavior: update event emission in `internal/platform/controller/controller_windows.go` and handling in `frontend/src/features/controller.js`.
- Add a status indicator: add backend status data in `App.GetStatuses`, emit `status:update` when it changes, then render it in a frontend module.
- Add Windows integration: implement it under the relevant `internal/platform/*` package, call it from `app.go` or an `App` method, and create a matching stub file.

## Verification

Useful checks before handing off feature work:

```sh
npm --prefix frontend run build
make build-linux
make build-win
```

Run the checks that fit the change and host OS. Ask the user before running test suites. Windows-only behavior needs real Windows validation, especially shell registration, Bluetooth pairing, XInput input, fullscreen restore, display mode changes, and shutdown/restart.
