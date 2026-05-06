# Project Guide

## Summary

This repository is a Wails v2 desktop launcher named `launcher`, branded in the UI as `StageBox`. It is built as a kiosk-style game launcher with a Go backend and a Vite-powered vanilla JavaScript frontend.

The app loads games from a configured game root, renders them as a fullscreen controller-friendly grid, launches selected game executables, hides/restores the launcher window around game sessions, and provides Windows kiosk helpers such as fullscreen mode, Bluetooth status/pairing, display resolution setup, wallpaper hosting, shell registration, shutdown, and restart.

The project is Windows-focused at runtime, but many platform-specific tools have non-Windows stubs so parts of the code can still compile or be inspected on Linux.

## Main Layout

- `main.go`: Wails entry point. Embeds `frontend/dist`, loads `config.json`, initializes logging, binds `App`, and starts Wails.
- `app.go`: Wails-bound backend API used by the frontend. Owns app lifecycle, game loading/launching, Bluetooth status events, controller bridge setup, and system actions.
- `internal/assets`: runtime asset helpers, including thumbnail cache path and Wails thumbnail HTTP handler.
- `internal/config`: JSON config shape. Currently `gamesRoot` and `debug`.
- `internal/games`: game metadata model, concurrent game directory loader, thumbnail generation/cache support, launch/kill tracking.
- `internal/jsonx`: strict JSON file loader used by config and game metadata loading.
- `internal/logx`: small logging package with file/stdout support and panic recovery.
- `internal/platform`: platform integration packages. Windows files contain Win32/PowerShell integrations; `*_stub.go` files keep non-Windows builds safe.
  - `bluetooth`: Bluetooth state, bring-up, and auto-pairing.
  - `controller`: XInput polling and Wails gamepad events.
  - `cursor`: cursor visibility helpers.
  - `display`: display resolution selection.
  - `shell`: per-user shell registration.
  - `wallpaper`: native wallpaper host.
  - `winapi`: shared Windows DLL handles.
- `frontend/src`: vanilla JS frontend modules and CSS.
- `frontend/src/features`: frontend feature modules for Bluetooth, clock, and controller navigation.
- `frontend/wailsjs`: generated Wails bindings used by frontend imports.
- `custom_scripts`: Windows bootstrap/autologon/Bluetooth helper scripts.
- `vendor`: committed Go vendor tree.

## Runtime Data Flow

1. `main.go` creates a default config and overlays `config.json` with `jsonx.LoadJSONFile`.
2. `NewApp` stores the config and Wails binds the `App` instance.
3. On startup, `App.startup`:
   - saves the Wails context,
   - tries to register the launcher as the per-user shell on Windows,
   - tries to force the primary display to 1920x1080 or the best lower mode,
   - starts a native wallpaper host for `loading.jpg`,
   - starts the XInput controller bridge,
   - starts Bluetooth status polling,
   - fullscreenes the Wails window.
4. `frontend/src/main.js` creates an empty grid, initializes controller navigation, subscribes to game loading events, then calls `LoadGames`.
5. `LoadGames` loads each game through an 8-worker pool. Each worker parses `lc.json`, hashes the source image, creates or reuses a 512px thumbnail under `cache/thumbnails`, and emits `game:loaded` when that game is ready.
6. Cards use `/game-thumbs/<game-id>-<hash>.jpg` URLs served by the Wails asset handler instead of base64 image payloads.
7. Activating a card calls `Launch(id)`. Go pauses controller navigation, emits `game:start`, hides the launcher, starts the game process, then restores fullscreen launcher state and emits `game:end` after exit.

## Game Metadata

Games are discovered by scanning immediate subdirectories of `config.gamesRoot`. Each game directory must contain `lc.json`.

Expected fields map to `internal/games.GameData`:

```json
{
  "executable": "Game.exe",
  "image": "cover.jpg",
  "name": "Game Name",
  "description": "Optional description",
  "showCursor": false
}
```

Notes:

- `id` is optional in JSON. If omitted, the loader creates a deterministic UUID from the game name, falling back to the game directory name when the name is blank. If `id` is supplied, it must be a valid UUID.
- JSON decoding is strict. Unknown keys fail because `jsonx.LoadJSONFile` uses `DisallowUnknownFields`.
- `executable`, `image`, and `startsOn` are resolved relative to the game directory.
- Images are processed into cached thumbnails. The frontend receives `thumbnailUrl` and `imageHash`, not base64 image data.
- JPEG, PNG, GIF, WebP, and AVIF source images are supported for thumbnail generation.

## Frontend Notes

- The frontend uses Vite and plain JavaScript, not a component framework.
- `frontend/index.html` contains the fixed topbar and `.content` shell.
- `frontend/src/main.js` renders the game grid dynamically.
- `frontend/src/features/controller.js` listens for Wails runtime events:
  - `pad:nav` with `{ dir: "left" | "right" | "up" | "down" }`
  - `pad:press` with `"A"` or `"KILL"`
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
- `Launch` is asynchronous from the UI perspective. It starts a goroutine that waits for the process and restores the launcher afterward.
- `KillRunning` only kills the tracked top-level process. It does not currently kill process trees.
- The XInput bridge emits Wails events and has pause/resume behavior so launcher navigation does not continue while a game is active.
- Bluetooth and wallpaper code use Win32 APIs directly. Keep Windows-only code behind `//go:build windows` and add or update matching stubs for other platforms.

## Build And Dev Commands

From the repo root:

```sh
make dev
make build-linux
make build-win
make dep
```

Command details:

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
- Do not assume Windows helper commands work on Linux. Non-Windows stubs intentionally return no-op or `"unknown"` behavior.
- Keep thumbnail filenames hash-based. The hash is the cache invalidation signal when source images change.

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
go test ./...
npm --prefix frontend run build
make build-linux
make build-win
```

Run the checks that fit the change and host OS. Windows-only behavior needs real Windows validation, especially shell registration, Bluetooth pairing, XInput input, fullscreen restore, display mode changes, and shutdown/restart.
