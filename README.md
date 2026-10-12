# StageBox

StageBox is a fullscreen Windows game launcher built with Wails, Go, and a Vite-powered JavaScript frontend. It is designed for kiosk-style living-room or cabinet setups where the launcher becomes the main shell, presents a controller-friendly game grid, and gets out of the way while a game or Steam session is running.

The project can be inspected and partially built on Linux, but the production runtime is Windows-focused because shell registration, Bluetooth pairing, Wi-Fi management, XInput, fullscreen restoration, shutdown, restart, and wallpaper hosting all use Windows APIs or Windows tools.

## Features

- Fullscreen game library with cached cover thumbnails.
- SDL3 controller navigation with normalized buttons, hotplug detection, and legacy input fallback, plus keyboard navigation.
- Per-game launch metadata through simple `lc.json` files.
- A shared launch-status screen for local and Steam games, with dismissible launch errors.
- Matching space-themed artwork behind the library cards and a native loading fallback during game transitions.
- Game process tracking with optional process-tree mode for launcher/bootstrapper games.
- Optional per-game controller-to-keyboard/mouse mappings.
- Optional delayed key press for games that open an intermediate launcher.
- Automatically lists installed Steam games alongside local games, with cached Steam artwork and game-specific advanced process tracking.
- Steam button that opens Steam if installed or downloads the installer when missing.
- RetroBat button that opens its EmulationStation frontend, or downloads and starts the official installer when missing.
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
  "debug": true,
  "retrobatPath": "D:\\RetroBat",
  "downloadControllerDatabase": true
}
```

`gamesRoot` points to the folder StageBox scans for local games. Installed Steam games are discovered separately, including Steam library folders on other drives; their editable launch settings are generated under `gamesRoot/SteamGames`. `debug` changes the exit behavior: in debug mode, the exit menu item quits the current launcher process; in normal mode, it exits back to Windows and coordinates with the service loop.

`retrobatPath` is optional and accepts the RetroBat installation folder or the full path to `RetroBat.exe`. Restart StageBox after editing `config.json`.

StageBox refreshes the community SDL controller database in the background on every Windows startup. `downloadControllerDatabase` defaults to `true`; set it to `false` to use the cached database without an internet request. Restart after changing this option. Keep manual edits in the separate overrides file described below.

## RetroBat

Use the RetroBat-logo button beside Steam in the top bar. Controller/keyboard navigation reaches it like the other topbar buttons. If RetroBat is missing, StageBox downloads the latest stable Windows 64-bit setup from [RetroBat's official releases](https://github.com/RetroBat-Official/retrobat/releases/latest), shows download progress, and runs the installer. Finish setup using the [official installation guide](https://wiki.retrobat.org/get-started/installation-and-first-startup), then press the button again to open RetroBat. Installation discovery refreshes on each press, including installations such as `D:\RetroBat`, so a StageBox restart is not required after setup. For a portable/custom location that is not found automatically, set `retrobatPath` in StageBox's `config.json` and restart StageBox.

With no configured path, StageBox checks `RETROBAT_PATH`, current-user and machine Windows App Paths/uninstall records in both registry views, `RetroBat` folders at the root of fixed/removable local drives (including `C:\RetroBat` and `D:\RetroBat`), Program Files/Local AppData, a `RetroBat` folder beside `stagebox.exe`, and `RetroBat.exe` on `PATH`. Network/CD drives and recursive disk scans are excluded. Discovered files must exist; stale registry entries are skipped. An explicit config/environment path is authoritative: a missing installation offers setup instead of opening another installation, while invalid executable names and access errors show a dismissible error.

The installer is saved as `cache/installers/RetroBatSetup.exe`. Downloads use temporary files and are checked against the official release's file size and Windows executable header before setup starts; failures remove partial files and preserve a previous completed download. Setup starts directly, with the same Explorer fallback used for Steam installers. StageBox stays visible while setup runs independently; the cursor is made visible for setup. Installation is interactive and does not automatically launch RetroBat afterward.

StageBox starts an installed `RetroBat.exe` from its installation folder, pauses launcher navigation, restores the normal cursor, and hides its window. RetroBat manages the ROM library, themes, overlays, emulator setup, controller mappings and emulator exit hotkeys. Returning from a game to EmulationStation keeps StageBox hidden; quit RetroBat to return to StageBox with fullscreen, focus, and input restored. Download, setup-start and session failures use the existing launch-error panel.

The Windows session tracks the launched process group, including EmulationStation and inherited emulator children, rather than ending when the bootstrap executable exits or the frontend hides during gameplay. A lingering child/helper keeps the session active until it exits. Processes launched by an unrelated broker, with elevation, or outside the group are not guaranteed to be tracked. If this RetroBat installation is already running outside StageBox, quit it first; StageBox refuses an untracked handoff. Closing StageBox leaves RetroBat running. StageBox's Start+Back game-kill action does not terminate RetroBat; use RetroBat's own exit controls.

This integration launches the RetroBat frontend; it does not import ROM cards. StageBox uses SDL3 for its own controller input. Opt-in virtual input for native games is a separate later step.

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

Every Steam game card launches with one command: `Steam.exe -gamepadui -fullscreen steam://run/<AppID>`. Steam handles its controller interface, sign-in, first-run agreements, updates and game startup. StageBox does not bootstrap or restart the client, check client/DLL registration, or choose a separate silent launch path. The panel shows **Launching Steam…** until the verified Steam interface or selected game takes over.

When a known update is needed, the panel offers **Open in Steam**, **Update in Steam**, and **Back to launcher**. Open in Steam uses the same game command and queues Play after the update. Update in Steam opens the game's library page without requesting Play: select Steam's Update button, then exit Big Picture (or hold Start+Back while no game has started) to return and play something else. StageBox never auto-launches after download completion in this mode and does not change Steam's download-while-playing settings. Closing Big Picture does not cancel a Play request already queued in Steam.

Steam's interface or a recognized setup dialog can stay open through a long download or setup. A client that never shows an interface has a two-minute startup grace. Once the selected game has a real window, StageBox follows only that game's processes using a dedicated Job Object and native handles. Start+Back stops the game without closing Steam or other games. After game exit, confirmed empty processes return after 750ms; live windowless processes retain a ten-second handoff grace.

For recognized foreground desktop Steam dialogs, the controller's left stick/D-pad moves the pointer, A clicks, and B sends Escape. Pointer fallback stops when the dialog loses focus or the game starts. Some external installers or Windows elevation prompts may still require a physical mouse/keyboard.

After a Steam card session ends, StageBox hides remaining client windows, asks Steam to leave Big Picture when that window exists, and reclaims native foreground focus before resuming launcher input. A brief background watcher hides late Steam UI; opening Steam again cancels it so Big Picture can reopen normally. Steam remains running for downloads and other background work. The topbar Steam button explicitly opens fullscreen Big Picture, including when Steam already runs in the tray; no persistent Steam settings are changed.

Steam cards show a white update icon before the title when the local Steam manifest reports an update is needed. The status refreshes every 15 seconds and after a session. Steam’s own interface shows download/install progress. StageBox logs manifest counters while starting; those are coarse snapshots that Steam may leave unchanged for minutes. This reflects updates already known to Steam; Steam may discover additional updates at launch.

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

Keep `steamAppId` unchanged. Supported settings are `name`, `description`, `image`, `controls`, `showCursor`, `waitAfterExitSeconds`, `delayedKey` and `keyMap`, with cursor/input fields using the same meanings as local game settings. Relative custom images resolve from the `SteamGames` folder. Omit a field to inherit its discovered default; explicit `false` and `0` override defaults. Steam manages launch options and StageBox retains its game-specific advanced tracking.

`steamSetup` is no longer supported. Remove that property from existing configs; unsupported fields cause a launch error. Every Steam game uses the same Big Picture command.

Existing files are never rewritten. Cursor/input edits apply on the next launch; reload StageBox to refresh card title/artwork/control edits. Configs are removed after uninstall and launcher reload when their manifests are absent. Downloading games, unavailable drives, missing Steam, or incomplete scans keep your files. Invalid configs remain on disk and show an error at launch.

Steam game tracking attaches after Steam creates the game process. Games started entirely through unrelated external brokers or protected/elevated processes can need additional tracking support; job assignment failures are logged and use handle tracking where possible. Check `stagebox.log` when a Steam game returns to the launcher unexpectedly.

Game crash-reporting helpers remain tracked for Start+Back cleanup, but their windows and process lifetime do not count as the game opening or staying active. A Crashpad helper can be present during normal play; its presence alone is not a crash diagnosis.

## Controller Input

StageBox uses SDL3's Gamepad API on Windows amd64. The official SDL 3.4.18 runtime and license are bundled into `stagebox.exe` and extracted to versioned files under `cache/assets`, so deploying the executable does not require installing SDL or copying another DLL. The runtime's built-in mappings handle supported Xbox, PlayStation, Nintendo and other controllers.

StageBox keeps the complete upstream database in `assets/gamecontrollerdb.txt` **beside `stagebox.exe`**, for example `C:\launcher\assets\gamecontrollerdb.txt`. On every startup, a separate Go goroutine downloads the current list from [SDL_GameControllerDB](https://github.com/mdqinc/SDL_GameControllerDB/blob/master/gamecontrollerdb.txt). Startup and controller polling do not wait for the request: StageBox immediately uses the cached database or SDL's built-in mappings. A successful validated download replaces the managed base file atomically; an identical download leaves it untouched, and failures retain the previous file. Its license is placed in the same assets folder. To disable the startup request, set `downloadControllerDatabase: false`.

Put your manual mappings in **`assets/gamecontrollerdb-overrides.txt`**. StageBox creates an empty commented template when missing and never overwrites it. This file loads after the base database, so your corrections survive every upstream refresh. The older `gamecontrollerdb.txt` directly beside the executable is still supported as an override source; the assets overrides file takes final precedence. The first local base file replaced by the new refresh policy is preserved as `assets/gamecontrollerdb-before-refresh.txt`, so edits made under the previous shared-file policy can be moved into the dedicated overrides file. Make future edits in the overrides file, since the base database is refreshed from upstream.

All three external sources are watched for changes. Valid changes normally take effect in about one to two seconds without restarting, including completed downloads, manual edits, replaced files and deleted entries. Save overrides as UTF-8 and use `platform:Windows` entries; invalid or unreadable changes keep the previous mappings. Reloading briefly rescans SDL controllers, releases held mapped keys/mouse buttons, and waits for neutral controller input before resuming. Batocera/Linux entries can have different GUIDs and button/axis layouts even for the same controller; connection mode and firmware can also affect the matching profile.

Precedence is SDL built-ins, downloaded/cached base database, legacy executable-adjacent overrides, then assets overrides. The Ultimate profile is present in the full upstream database. No separate StageBox mapping subset is embedded, extracted or loaded from the runtime cache. Old `cache/assets/stagebox-gamecontrollerdb-v1.txt` and its license can be deleted; the current build never recreates them. The full upstream database replaces matching built-in profiles too, so check both controllers after updating.

One selected controller drives StageBox, retained until it disconnects. The controller indicator counts opened SDL gamepads. Detection refreshes while the launcher is paused for games; losing/changing a device releases mapped keys/buttons, resets repeat/kill state and requires neutral sticks, released triggers and buttons before accepting the replacement controller. Returning from a game still uses the existing launcher foreground/neutral gate.

If SDL cannot load/initialize, StageBox logs the error and uses XInput/WinMM. When SDL has no mapped gamepads, the same native fallback can still handle a legacy joystick. SDL and native input are not combined for the same navigation frame. For troubleshooting, start StageBox with `STAGEBOX_CONTROLLER_BACKEND=legacy` to use the prior backend; remove that environment variable to use SDL3 again. SDL's device inventory can still include distinct virtual devices created by other controller software; no universal physical/virtual identity merge is claimed.

SDL reads controllers, including native XInput controllers, for StageBox and its existing keyboard/mouse mappings. Games still read their controllers independently through Windows or their own input libraries. StageBox's SDL bridge does not forward the controller to the game, create a virtual Xbox controller or add `enableVirtualInput`. Steam and RetroBat continue managing their game input; native-game virtual translation and the hold-A card menu belong to step 3.

## How To Use

1. Start StageBox.
2. Wait for the game grid to load local games and installed Steam games.
3. Navigate with a controller D-pad/left stick or keyboard arrows.
4. Press the controller `A` button or `Enter` to launch the selected game.
5. StageBox shows a cinematic launch panel with the game's cached artwork, a green glow, and “Starting game…” until a visible game or intermediate launcher window appears, then hides and restores itself after the tracked session exits. Missing artwork uses an atmospheric moon-and-forest backdrop. Steam launches also show connection/update progress, with percentages only when available. Cursor settings, controller mapping targets, and delayed-key timing take effect at the window handoff.
6. Open the tools menu for Bluetooth pairing, Wi-Fi networks, reloading games, opening the games folder, restart, power off, or exiting the launcher.
7. Use the Steam button in the top bar to open Steam Big Picture for controller navigation. If Steam is not found, StageBox downloads and starts the official Steam installer.
8. Open Sound Settings from the tools menu to choose an output device. While StageBox is focused, right-stick up/down adjusts volume; the speaker indicator and temporary volume bar show the current level. Right-stick volume control is disabled during games. Keyboard multimedia volume/mute keys work system-wide, including games, without bringing StageBox to the foreground.
9. Use the RetroBat button in the top bar to open your configured RetroBat installation. Quit RetroBat to return to StageBox.

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
- For controller problems, inspect `controller backend SDL3 initialized`, `SDL3 controller mappings loaded`, `SDL3 joystick discovered`, `SDL3 gamepad mapping`, `SDL3 gamepad connected/disconnected`, and `controller input device changed` in `stagebox.log`. Joystick discovery includes name, VID/PID and mapped status, even when the device falls back to native input. SDL extraction/load/init/discovery errors log the fallback reason. Check the cache is writable, try the legacy backend, and verify disconnect/reconnect plus game return on the target Windows machine.

## Publishing Notes

Before making the repository public, add a top-level `LICENSE` file and update this section with the selected license. The bundled Nunito font files include their own SIL Open Font License text under `frontend/src/assets/fonts/OFL.txt`. The Steam vector icon comes from [Simple Icons](https://github.com/simple-icons/simple-icons/blob/develop/icons/steam.svg), distributed under [CC0](https://github.com/simple-icons/simple-icons/blob/develop/LICENSE.md).

The bundled [SDL3 runtime](https://github.com/libsdl-org/SDL/releases/tag/release-3.4.18) is distributed under its zlib license, retained at `internal/platform/controller/sdl/windows_amd64/LICENSE.txt` and extracted beside its cached runtime. Release URLs and verified SHA-256 digests are recorded in `internal/platform/controller/sdl/README.md`.

The downloaded controller database comes from [SDL_GameControllerDB](https://github.com/mdqinc/SDL_GameControllerDB) under its zlib license, retained at `internal/platform/controller/sdl/gamecontrollerdb-LICENSE.txt` and copied to the executable-adjacent assets folder. StageBox embeds its license only; it does not embed a separate controller database.
