# StageBox

StageBox is a fullscreen Windows game launcher built with Wails, Go, and a Vite-powered JavaScript frontend. It is designed for kiosk-style living-room or cabinet setups where the launcher becomes the main shell, presents a controller-friendly game grid, and gets out of the way while a game or Steam session is running.

The project can be inspected and partially built on Linux, but the production runtime is Windows-focused because shell registration, Bluetooth pairing, Wi-Fi management, XInput, fullscreen restoration, shutdown, restart, and wallpaper hosting all use Windows APIs or Windows tools.

## Features

- Fullscreen game library with cached cover thumbnails.
- Controller and keyboard navigation for the launcher UI.
- Per-game launch metadata through simple `lc.json` files.
- A shared launch-status screen for local and Steam games, with dismissible launch errors.
- Matching space-themed artwork behind the library cards and a native loading fallback during game transitions.
- Game process tracking with optional process-tree mode for launcher/bootstrapper games.
- Optional per-game controller-to-keyboard/mouse mappings.
- Optional delayed key press for games that open an intermediate launcher.
- Automatically lists installed Steam games alongside local games, with cached Steam artwork and game-specific advanced process tracking.
- Steam button that opens Steam if installed or downloads the installer when missing.
- Sound Settings for selecting the Windows default output device, with current-output highlighting, master volume, and mute controls.
- Bluetooth status indicator and Windows pairing wizard integration.
- Wi-Fi scan/connect overlay for kiosk setups without the Windows shell.
- Controller count, Bluetooth, Wi-Fi, and clock indicators in the top bar.
- Windows kiosk helpers for shell registration, autologon setup, service priming, restart, shutdown, and exit-to-Windows.
- Log files for app and service-loop diagnostics.

## Requirements

### Runtime

- Windows 10 or Windows 11.
- Microsoft Edge WebView2 Runtime.
- A folder containing local games with one `lc.json` per game, or an installed Steam library.
- Administrator access if you want to install StageBox as a kiosk shell with the provided PowerShell installer.

### Development

- Go matching the version in `go.mod`.
- Node.js and npm for the Vite frontend.
- Wails v2 CLI.
- Windows build tooling when building on Windows.
- MinGW-w64 when cross-building the Windows executable from Linux.
- Linux Wails dependencies if using `make dev` or `make build-linux` on Linux.

## Install From Source

Clone the repository and install frontend dependencies:

```sh
git clone <repo-url>
cd launcher
npm --prefix frontend install
```

Install the Wails CLI if it is not already available:

```sh
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

Build the Windows release bundle:

```sh
make release
```

The release bundle is written to `release/` and includes:

- `stagebox.exe`
- `install-stagebox.ps1`
- `svc.vbs`
- `systemsvcs.ps1`
- `elevated-PS.cmd`

For a kiosk installation, move the release bundle to the Windows machine, open PowerShell as Administrator from that folder, and run:

```powershell
.\install-stagebox.ps1
```

The installer stages files under `C:\launcher`, writes a default `config.json` when needed, registers StageBox as the user shell, sets up helper scheduled tasks, and prepares Wi-Fi/Bluetooth services for kiosk use.

## Manual Runtime Setup

If you only want to run the app without installing it as the Windows shell, place `stagebox.exe` in a folder with a `config.json` file:

```json
{
  "gamesRoot": "C:\\games",
  "debug": true
}
```

`gamesRoot` points to the folder StageBox scans for local games. Installed Steam games are discovered separately, including Steam library folders on other drives; their editable launch settings are generated under `gamesRoot/SteamGames`. `debug` changes the exit behavior: in debug mode, the exit menu item quits the current launcher process; in normal mode, it exits back to Windows and coordinates with the service loop.

## Game Library Format

StageBox scans the immediate subfolders under `gamesRoot`. Each game folder must contain an `lc.json` file.

Example:

```text
C:\games
  Halo
    lc.json
    Halo.exe
    cover.jpg
  Racing Game
    lc.json
    game.exe
    artwork.png
```

Minimal `lc.json`:

```json
{
  "name": "Halo",
  "executable": "Halo.exe",
  "image": "cover.jpg"
}
```

Fuller example:

```json
{
  "name": "Modern Warfare 3",
  "description": "Optional description",
  "executable": "Game.exe",
  "args": ["-fullscreen"],
  "startsOn": ".",
  "image": "cover.jpg",
  "controls": 2,
  "trackMode": "processTree",
  "advancedTracking": false,
  "waitAfterExitSeconds": 1,
  "showCursor": false,
  "delayedKey": {
    "seconds": 4,
    "key": "Enter"
  },
  "keyMap": {
    "mouse": "rightStick",
    "mouseSensitivity": 10,
    "buttons": {
      "A": ["Space", "Enter"],
      "B": ["Escape"],
      "RT": ["MouseLeft"]
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

Supported image inputs include JPEG, PNG, GIF, WebP, and AVIF. StageBox generates cached JPEG thumbnails under `cache/thumbnails`.

Important metadata notes:

- JSON is strict. Unknown fields will make that game fail to load.
- `id` is optional. If omitted, StageBox creates a deterministic UUID from the game name or folder name.
- Relative `executable`, `image`, and `startsOn` values are resolved from the game folder.
- `controls` controls the card icons: omit or use `0` for controller, `1` for keyboard/mouse, and `2` for both.
- `trackMode` can be omitted or set to `process` for normal process waiting. Use `processTree` for launchers that spawn the real game as a child process.
- `advancedTracking` is optional and defaults to `false`. Set it to `true` on Windows for games whose launcher exits before the real game: StageBox attaches the initial executable to a Job Object before it runs, follows its child processes, and makes the kill shortcut terminate the group. It overrides `trackMode` for that game. A 30-second window-start/handoff grace and a 2-second window-free quiet period prevent invisible helpers from keeping the session open indefinitely; `waitAfterExitSeconds` adds a final delay. It cannot track a game started by an unrelated external service or already-running launcher. Disable it if a game is incompatible with Job Objects. Closing StageBox does not automatically kill the group.
- `showCursor` should be enabled for games or launchers that need the Windows cursor visible.
- `keyMap` maps gamepad input to keyboard or mouse input while the game is running. See `keypad-sample.json` for a working sample.

## Steam Library

StageBox automatically finds Steam through its Windows installation registry entries, `STEAM_PATH`/`STEAM_DIR`, or standard Program Files locations. It scans Steam's configured library folders and adds games with a fully installed manifest and files present on disk. Uninstalled games, unfinished downloads, and Steamworks redistributables are excluded. Available offline Steam metadata also filters out tools and supplies the card icons: full controller support shows a controller, partial support shows controller and keyboard/mouse, and no declared support shows keyboard/mouse. Missing metadata leaves icons hidden. Reload the launcher after installing or removing games.

Steam cards have a folded upper-right corner with the Steam logo. Cards use Steam's local artwork cache first, including newer content-hash folders. Missing artwork is downloaded from Steam and cached for later offline use; if artwork is unavailable, the game remains listed with a Steam icon. No account API key is required.

Launching a Steam card first waits for a live Steam client with stable client/DLL registration. StageBox starts Steam silently when closed, or sends one silent reactivation if an existing client's registration remains unavailable. It shows “Starting Steam…”, “Waiting for Steam…” or “Connecting to Steam…” during these stages. It then asks Steam to run that game, preserving Steam launch options, DRM, and first-run setup. StageBox tracks the selected game's processes with a dedicated Job Object and native handles; it returns when the game closes even if Steam stays open. Start+Back stops the tracked game without closing Steam or other library games. StageBox keeps a launch-status panel visible until the actual game window opens. Ordinary startup allows two minutes for a game window. Known unpaused updates, including queued updates without byte totals, have a separate 30-minute wait without new manifest data; changed counters/stages renew it. After an update completes, the game gets a fresh two-minute startup window. Paused or stalled updates show a specific error; StageBox does not cancel Steam's download or queued launch when its wait ends. After startup, an empty tracked game process set returns after a 750ms debounce; windowless live processes keep a ten-second handoff grace.

When an update is already known, StageBox requests installation/download first and waits before sending the game launch. The update panel offers **Back to launcher** (controller A/B, Enter/Escape or click) so you can choose another game while Steam retains the download. Staying in the panel launches after completion. An already active update is reused. Steam may show a confirmation for the install request; StageBox does not cancel downloads or change Steam’s download-while-playing settings. Updates first discovered after Steam receives a game launch still use the existing waiting panel.

If an established client's registration keeps pointing to a dead process, StageBox attempts one graceful Steam restart while showing “Reconnecting to Steam…”. It checks Steam's tracked process tree and all library manifests for games/downloads first, waits for Steam to exit normally, then starts it silently and waits for fresh registration. Unrelated desktop and driver processes are not inspected. Games started through independent external brokers without tracked Steam ancestry or busy manifest flags may escape this activity check. Busy or uncertain state and a refused shutdown produce a specific error; StageBox never forcibly terminates Steam to repair registration. Healthy clients remain running as usual after games close.

After a Steam card session ends, StageBox hides remaining client windows, asks Steam to leave Big Picture when that window exists, and reclaims native foreground focus before resuming launcher input. A brief background watcher hides late Steam UI; opening Steam again cancels it so Big Picture can reopen normally. Steam remains running for downloads and other background work. The topbar Steam button explicitly opens fullscreen Big Picture, including when Steam already runs in the tray; no persistent Steam settings are changed.

Steam cards show a white update icon before the title when the local Steam manifest reports an update is needed. The status refreshes every 15 seconds and after a session. Launch feedback shows downloading/installing progress when Steam supplies byte counters. Generic update startup with no transferred bytes shows preparing; known byte totals alone do not imply an active download or completed installation. After download completion, partial staging bytes show installation progress. Percentages describe the last reported phase snapshot; Steam may leave these counters unchanged for minutes. This reflects updates already known to Steam; Steam may discover additional updates at launch.

Steam card launches require internet access. If StageBox cannot verify access, it keeps the launcher visible and shows an internet-required message; dismiss it with controller A/B/Start or Enter/Escape. Local games and the topbar Steam button retain their existing behavior.

### Per-game Steam settings

On first discovery, StageBox creates `gamesRoot/SteamGames/<AppID>-lc.json` with the game name and defaults. Edit that file to override cursor/input settings, for example:

```json
{
  "steamAppId": 570,
  "name": "Dota 2",
  "showCursor": false,
  "controls": 0,
  "waitAfterExitSeconds": 0,
  "delayedKey": null,
  "keyMap": null
}
```

Keep `steamAppId` unchanged. Supported settings are `name`, `description`, `image`, `controls`, `showCursor`, `waitAfterExitSeconds`, `delayedKey`, and `keyMap`, using the same meanings as local game settings. Relative custom images resolve from the `SteamGames` folder. Omit a field to inherit its discovered default; explicit `false` and `0` override defaults. Steam manages launch options and StageBox retains its game-specific advanced tracking.

Existing files are never rewritten. Cursor/input edits apply on the next launch; reload StageBox to refresh card title/artwork/control edits. Configs are removed after uninstall and launcher reload when their manifests are absent. Downloading games, unavailable drives, missing Steam, or incomplete scans keep your files. Invalid configs remain on disk and show an error at launch.

Steam game tracking attaches after Steam creates the game process. Games started entirely through unrelated external brokers or protected/elevated processes can need additional tracking support; job assignment failures are logged and use handle tracking where possible. Check `stagebox.log` when a Steam game returns to the launcher unexpectedly.

Game crash-reporting helpers remain tracked for Start+Back cleanup, but their windows and process lifetime do not count as the game opening or staying active. A Crashpad helper can be present during normal play; its presence alone is not a crash diagnosis.

## How To Use

1. Start StageBox.
2. Wait for the game grid to load local games and installed Steam games.
3. Navigate with a controller D-pad/left stick or keyboard arrows.
4. Press the controller `A` button or `Enter` to launch the selected game.
5. StageBox shows a cinematic launch panel with the game's cached artwork, a green glow, and “Starting game…” until a visible game or intermediate launcher window appears, then hides and restores itself after the tracked session exits. Missing artwork uses an atmospheric moon-and-forest backdrop. Steam launches also show connection/update progress, with percentages only when available. Cursor settings, controller mapping targets, and delayed-key timing take effect at the window handoff.
6. Open the tools menu for Bluetooth pairing, Wi-Fi networks, reloading games, opening the games folder, restart, power off, or exiting the launcher.
7. Use the Steam button in the top bar to open Steam Big Picture for controller navigation. If Steam is not found, StageBox downloads and starts the official Steam installer.
8. Open Sound Settings from the tools menu to choose an output device. While StageBox is focused, right-stick up/down adjusts volume; the speaker indicator and temporary volume bar show the current level. Right-stick volume control is disabled during games. Keyboard multimedia volume/mute keys work system-wide, including games, without bringing StageBox to the foreground.

The topbar Steam session follows existing and replacement clients from the same executable path and Windows session. After Big Picture appears, StageBox returns after 750ms without Big Picture/game windows when no tracked game process remains; full Steam shutdown uses a 1.5-second debounce. Windowless live game processes retain a ten-second handoff grace. Closing to the tray or exiting Big Picture into desktop Steam also returns to StageBox. Remaining client windows are hidden and StageBox reclaims foreground focus before input resumes. Sign-in/update windows remain usable during startup, with a two-minute limit if a live client shows no interface. Steam is not placed in a Job Object and its kill-shortcut behavior is unchanged. Mid-session client restarts or completely empty game handoffs taking longer than the short debounce, changed Steam window identities, or games started through unrelated processes may need further tracking adjustments.

## Development

Common commands from the repository root:

```sh
make dev
make build-linux
make build-win
make release
make dep
```

Command summary:

- `make dev` starts Wails development mode.
- `make build-linux` builds a Linux Wails binary for local development checks.
- `make build-win` cross-builds `build/bin/stagebox.exe`.
- `make release` creates the Windows staging bundle in `release/`.
- `make dep` runs `go mod tidy` and refreshes `vendor/`.

Frontend-only commands:

```sh
npm --prefix frontend run dev
npm --prefix frontend run build
npm --prefix frontend run preview
```

## Logs And Troubleshooting

- `stagebox.log` is written in the launcher's working directory and preserved across restarts. Its absolute path is logged at startup. At startup, logs of at least 8MiB rotate to `stagebox.log.previous`, replacing the previous backup; a single long run can grow beyond that size.
- Logging always includes DEBUG messages. `config.json`'s `debug` flag controls exit behavior, not log verbosity.
- For intermittent Steam launch failures, capture the log around `Steam game requested`, `Steam readiness snapshot policy=registered-stable-v3`, `Steam stale registration recovery`, `Steam game startup status`, and `Steam game process exited`. These show registration, client age/access context, recovery results, update flags/counters and native exit status; stable registration does not guarantee successful Steam API initialization or sign-in.
- `svc.log` is written by the service/helper scripts on Windows.
- If no local games appear, check that `config.json` points to a readable `gamesRoot` and that each game folder contains a valid `lc.json`. For missing Steam games, check Steam is installed, the library drive is available, and downloads have finished; reload StageBox and inspect its Steam discovery logs.
- If a game opens another launcher and StageBox returns too early, try `"trackMode": "processTree"`.
- If Wi-Fi scanning fails in kiosk mode, rerun `install-stagebox.ps1` as Administrator and inspect `svc.log`.
- If the screen is black after shell startup, inspect `stagebox.log` for frontend boot and render markers.

## Publishing Notes

Before making the repository public, add a top-level `LICENSE` file and update this section with the selected license. The bundled Nunito font files include their own SIL Open Font License text under `frontend/src/assets/fonts/OFL.txt`. The Steam vector icon comes from [Simple Icons](https://github.com/simple-icons/simple-icons/blob/develop/icons/steam.svg), distributed under [CC0](https://github.com/simple-icons/simple-icons/blob/develop/LICENSE.md).
