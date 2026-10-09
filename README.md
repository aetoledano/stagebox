# StageBox

StageBox is a fullscreen Windows game launcher built with Wails, Go, and a Vite-powered JavaScript frontend. It is designed for kiosk-style living-room or cabinet setups where the launcher becomes the main shell, presents a controller-friendly game grid, and gets out of the way while a game or Steam session is running.

The project can be inspected and partially built on Linux, but the production runtime is Windows-focused because shell registration, Bluetooth pairing, Wi-Fi management, XInput, fullscreen restoration, shutdown, restart, and wallpaper hosting all use Windows APIs or Windows tools.

## Features

- Fullscreen game library with cached cover thumbnails.
- Controller and keyboard navigation for the launcher UI.
- Per-game launch metadata through simple `lc.json` files.
- Game process tracking with optional process-tree mode for launcher/bootstrapper games.
- Optional per-game controller-to-keyboard/mouse mappings.
- Optional delayed key press for games that open an intermediate launcher.
- Steam button that opens Steam if installed or downloads the installer when missing.
- Bluetooth status indicator and Windows pairing wizard integration.
- Wi-Fi scan/connect overlay for kiosk setups without the Windows shell.
- Controller count, Bluetooth, Wi-Fi, and clock indicators in the top bar.
- Windows kiosk helpers for shell registration, autologon setup, service priming, restart, shutdown, and exit-to-Windows.
- Log files for app and service-loop diagnostics.

## Requirements

### Runtime

- Windows 10 or Windows 11.
- Microsoft Edge WebView2 Runtime.
- A folder containing your games and one `lc.json` file per game.
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

`gamesRoot` points to the folder StageBox scans for games. `debug` changes the exit behavior: in debug mode, the exit menu item quits the current launcher process; in normal mode, it exits back to Windows and coordinates with the service loop.

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

## How To Use

1. Start StageBox.
2. Wait for the game grid to load from `gamesRoot`.
3. Navigate with a controller D-pad/left stick or keyboard arrows.
4. Press the controller `A` button or `Enter` to launch the selected game.
5. StageBox hides while the game is running and restores itself after the tracked process exits.
6. Open the tools menu for Bluetooth pairing, Wi-Fi networks, reloading games, opening the games folder, restart, power off, or exiting the launcher.
7. Use the Steam button in the top bar to open Steam. If Steam is not found, StageBox downloads and starts the official Steam installer.

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

- `stagebox.log` is written by the launcher and is truncated on each start.
- `svc.log` is written by the service/helper scripts on Windows.
- If no games appear, check that `config.json` points to a readable `gamesRoot` and that each game folder contains a valid `lc.json`.
- If a game opens another launcher and StageBox returns too early, try `"trackMode": "processTree"`.
- If Wi-Fi scanning fails in kiosk mode, rerun `install-stagebox.ps1` as Administrator and inspect `svc.log`.
- If the screen is black after shell startup, inspect `stagebox.log` for frontend boot and render markers.

## Publishing Notes

Before making the repository public, add a top-level `LICENSE` file and update this section with the selected license. The bundled Nunito font files include their own SIL Open Font License text under `frontend/src/assets/fonts/OFL.txt`.
