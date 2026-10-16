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
- Bluetooth status indicator, Windows pairing wizard, and a paired-device list with Forget.
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
- Optional virtual Xbox input needs Windows x64, the bundled `assets/hidmaestro` folder and administrator permission for device creation. Its .NET runtime is included.

### Development

- Go matching the version in `go.mod`.
- Node.js and npm for the Vite frontend.
- Wails v2 CLI.
- Windows build tooling when building on Windows.
- MinGW-w64 when cross-building the Windows executable from Linux.
- .NET 10 SDK and Python 3.11+ for building the bundled Windows virtual-controller helper. Initial builds download the pinned HIDMaestro SDK and .NET runtime packages.
- Linux Wails dependencies if using `make dev` or `make build-linux` on Linux.

The Windows build now compiles the C# helper too, so the development machine needs the **SDK**, even though Windows runtime users get .NET bundled. On Fedora, install it with:

```sh
sudo dnf install dotnet-sdk-10.0
```

See [Microsoft's Fedora instructions](https://learn.microsoft.com/en-us/dotnet/core/install/linux-fedora) for supported distributions. Check build prerequisites with `make virtual-helper-deps` or see `make help`. If your SDK is installed outside `PATH`, use `make DOTNET=/absolute/path/to/dotnet`; no shell-profile changes are required. `make` checks Python and the selected SDK before downloading helper assets.

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
- `assets/hidmaestro/` (portable helper, runtime, SDK and licenses)
- `install-stagebox.ps1`
- `install-hidhide.ps1`
- `svc.vbs`
- `systemsvcs.ps1`
- `elevated-PS.cmd`

For a kiosk installation, move the release bundle to the Windows machine, open PowerShell as Administrator from that folder, and run:

```powershell
.\install-stagebox.ps1
```

The installer stages files under `C:\launcher`, installs HidHide before shell/task registration, writes a default `config.json` when needed, registers StageBox as the user shell, sets up helper scheduled tasks, and prepares Wi-Fi/Bluetooth services for kiosk use. An existing HidHide installation is preserved. Restart Windows once after a new driver installation; the script never restarts the machine automatically.

For an existing StageBox installation, run only the driver script from PowerShell as Administrator:

```powershell
.\install-hidhide.ps1
```

Keep the bundled `HidHide_1.5.230_x64.exe` beside that script or under `assets\hidmaestro`. It also searches `C:\launcher\assets\hidmaestro`; use `-LauncherDir` for a different StageBox folder or `-InstallerPath` for an explicit installer. The standalone script logs to `hidhide-install.log` beside itself. It verifies the official pinned SHA-256, installs silently with automatic restart disabled, and leaves existing HidHide device/application lists unchanged. After a new install, restart Windows before using virtual input. The complete StageBox installer invokes the same script and records its output in `installation.log`.

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

This integration launches the RetroBat frontend; it does not import ROM cards. StageBox uses SDL3 for its own controller input. Controllers saved as Virtual Xbox are passed through StageBox’s SDL-to-Xbox bridge for the entire RetroBat session; Native controllers stay native. See Controller Input below.

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
  "enableVirtualInput": false,
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
- `enableVirtualInput` is optional and defaults to `false`. Enable it manually for native games that require XInput; the card menu saves this field and launch re-reads it without requiring a library reload.
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

SDL reads controllers, including native XInput controllers, for StageBox and its existing keyboard/mouse mappings. Native-default controllers remain available to games, Steam and RetroBat directly. Virtual-default controllers are normalized by StageBox’s SDL backend and forwarded as Xbox controllers for those sessions.

### Forgetting a Bluetooth device

Open **Tools → Bluetooth Devices**, select the device by name/address and choose **Forget device**. Up/Down moves through the list; Left/Right moves between actions. Cancel returns to the same device. This removes its Windows pairing, and a connected device disconnects. Keep another controller, keyboard or mouse available. Use **Pair device** afterward to open the existing Windows pairing wizard.

The list covers Classic and Bluetooth LE devices paired directly with Windows, including the Ultimate 2C over direct Bluetooth LE. LE rows are labeled **Bluetooth LE**. A paired LE device can remain listed while disconnected even if Windows omits its address; it shows **Address unavailable**. If one Bluetooth backend fails, available devices remain visible with a warning. Pairings stored by USB adapters, including 8BitDo adapters, are managed by the adapter and are not Windows pairing records. Forgetting a Windows pairing also removes saved profiles linked to that device from `assets/controllers.json`. If Windows supplies no address to link the pairing to its profile, the device is still forgotten and StageBox reports that saved-profile cleanup could not be verified; remove that stale entry through Controllers after unpairing. Shared SDL mapping overrides remain available. Windows PowerShell hosts the built-in Windows Runtime API for LE enumeration/unpairing; its script is embedded in StageBox, with no extra helper file to copy.

### Controller defaults and mapping test

When a new mapped non-XInput controller connects, StageBox asks whether to use it as **Native** or **Virtual Xbox**. Choose Native for a controller the game supports directly, such as a DualSense; choose Virtual Xbox for a generic pad that needs Xbox input. The choice is saved in `assets/controllers.json` beside StageBox and reused on reconnect. Native XInput controllers automatically stay native.

Unknown-controller prompts retry once StageBox owns foreground focus, including after startup or pairing; they wait while another menu, game or pairing screen is active. To reset saved defaults, close StageBox and remove `<StageBox folder>/assets/controllers.json` before restarting. Reconnected non-XInput pads ask again; native XInput pads remain automatic. The log records the exact preferences path and loaded profile count.

Every connected SDL gamepad can navigate the launcher, including menus and right-stick volume. Each has independent button/repeat state; a controller connected with a held button must return to neutral before it can navigate. The topbar count uses connected SDL gamepads, excluding saved disconnected profiles. If SDL is unavailable or has no mapped controllers, StageBox uses its native XInput/WinMM fallback instead. Game-key/pointer helpers and controller diagnostics retain their selected controller.

Connection labels use Windows device ancestry when available and SDL's wired/wireless state otherwise. Bluetooth includes recognized Classic/LE device ancestry; an unrecognized path says **Unknown** instead of assuming USB. **Wireless** alone does not distinguish Bluetooth from another radio link. **USB / wireless receiver** describes a USB receiver when SDL reports a wireless controller behind it. Display-label corrections preserve existing profile IDs/defaults.

The topbar Bluetooth icon turns green for a connected Classic or Bluetooth LE controller, including the Ultimate 2C. A saved pairing alone does not light it, and USB receivers do not count as direct Bluetooth. Connection changes appear on the normal two-second status refresh.

Open **Start → Controllers** to change a saved default or select **Test controller mapping**. Each controller name includes a short ID so identical model names can be distinguished; the ID is also shown in its settings and test panel. The test shows a generic controller diagram with blank face buttons, live positional highlights, sticks and triggers. Face-button readouts use South/East/West/North rather than printed labels. Launcher actions pause while controls are grabbed. Hold Select+Start for one second to release the grab and stay on the test screen; use the D-pad and A to navigate its buttons. Select **Grab controls** to resume input testing. Use **Back to controller settings**, the mouse Back button or keyboard Escape to leave. If the tested controller disconnects, StageBox releases the grab automatically so another controller or the keyboard can navigate and exit; reconnecting does not grab it again. The highlights show StageBox's normalized button positions, which can differ from the labels printed on a Nintendo-style pad.

Open **Input details** for raw SDL trigger/axis readings and mapping sources. The mapping test also has a **Test rumble** button: release the grab with Select+Start, then select it using the D-pad and A for a short pulse on the selected physical controller. Mouse and keyboard Tab/Enter also work. SDL errors appear in the panel; leaving the test stops the pulse. This checks physical SDL rumble separately from virtual game feedback. Trigger diagnostics show the active button/axis bindings and raw values. Digital trigger bindings can only report 0% or 100%; pressure requires analog input from the controller’s current mode.

Virtual-default pads convert in native games, Steam from the topbar, direct Steam card launches and RetroBat. The game's hold-A switch additionally converts Native-default pads, so turning it off leaves Virtual-default pads virtual. This allows a supported DualSense to remain native alongside a generic virtual Xbox pad. Steam and RetroBat use only the controller’s saved Native/Virtual choice; there is no per-game switch for those sessions. Configure the pads before launching; defaults cannot be changed during a game.

USB/Bluetooth modes can have separate identities. Without a reported serial number, StageBox uses the Windows interface path, so changing ports can require choosing a default again. Keep `assets/controllers.json` when updating StageBox. Profiles and the mapping test require SDL; legacy fallback provides navigation only.

To clean up old entries, select a disconnected controller and choose **Remove saved controller**, then confirm. A Bluetooth controller that is still paired must be removed through **Bluetooth Devices → Forget device** instead. Older profiles gain a verified Bluetooth address when their controller reconnects and Windows exposes the association. Bluetooth or uncertain-transport profiles without that address cannot be removed while any Windows Bluetooth pairing remains because StageBox cannot verify which device owns the profile. The Controllers list scrolls with mouse, keyboard or controller while Back stays visible.

### Optional virtual Xbox input

For a native game that needs or works better with Xbox/XInput controllers, select its card and **hold A for one second** to open Game Options. Turn **Virtual Xbox controller** on, then press B to return. A short A press launches on release. Right-click, the context-menu key or Shift+F10 also opens the native card menu. Enabled cards show a compact **XInput** wordmark in StageBox’s accent color, with no background and a small shadow. The option saves `"enableVirtualInput": true` in that game's `lc.json`; omitted/false uses each controller's saved default. Turning it on additionally converts Native-default non-XInput controllers; Virtual-default controllers already use Xbox input. Steam cards and the Steam/RetroBat topbar sessions use the controller defaults directly; this per-game switch applies only to native cards.

Copy the whole `assets/hidmaestro` directory beside `stagebox.exe`, keeping it paired with the executable from the same build. The bundle includes the helper, SDK, drivers/tools and a self-contained .NET runtime, so no separate .NET or ViGEmBus installation is needed. Windows still requires UAC approval when a session needs virtual devices. First use registers HIDMaestro's driver packages and locally trusted signing certificate; those stay installed after the session’s virtual devices are removed. See [HIDMaestro's driver documentation](https://hidmaestro.org/docs/reference/driver-install-and-signing/).

Virtual input also uses **HidHide** to hide each converted physical controller from the game, preventing duplicate physical/virtual input. StageBox is temporarily allowlisted so SDL can keep reading it. The official HidHide 1.5.230 installer is included in the helper folder and installed by `install-stagebox.ps1` or the standalone `install-hidhide.ps1`; restart Windows once after a new install. Game launch only checks its availability and directs you to setup if it is missing. Existing installations are reused. [HidHide](https://github.com/nefarius/HidHide/releases/tag/v1.5.230.0) is a Windows filter driver, so its one-time system installation is required even with the portable bundle.

StageBox verifies that the helper is denied access to the physical controller and StageBox can still open it before starting the game. It then forwards standard buttons, D-pad, sticks and triggers to an independent virtual Xbox 360 controller for each converted pad. On return it removes its virtual devices and restores the HidHide changes it made, preserving existing entries. A leftover `assets/hidhide-session.json` is recovered before controller polling on the next startup if the helper was forcibly terminated. Keep this recovery file until StageBox has restored it. Verified native SDL `XInput#` sources bypass virtual creation and hiding. Sessions with no controllers requiring conversion do not begin hiding. Steam/RetroBat virtual forwarding starts before their executable is launched and lasts through the tracked session, including games/emulators started inside them. Disconnects send neutral input and allow the same physical path to reconnect; the virtual pad cannot become its own source. Missing bundles, cancelled UAC or setup failures leave the game/frontend unopened with an error.

This integration supports up to four converted controllers, subject to the four total Windows XInput slots including native Xbox pads. Connect your intended controllers before launching: the converted roster is fixed for the session and new unrelated pads are deferred until return; ordinary two-motor rumble is forwarded to the matching physical SDL controller when supported. Analog trigger input retains the 0–255 Xbox range; digital source bindings remain on/off; adaptive-trigger effects are not exposed by the virtual Xbox 360 profile. HidHide's application list must use normal mode. StageBox refuses an inverted list or enabling a disabled list that would also hide unrelated configured devices. HidHide does not block Raw Input, so software using that API can still see the physical device. See its [FAQ](https://docs.nefarius.at/projects/HidHide/FAQ/). The user verified virtual forwarding in Blur; the user also verified physical hiding in a game that previously received double input. Mixed and multiple-controller sessions still need Windows validation.

### Steam PS/Guide button prompts

While StageBox is idle and focused, a PS/Guide press arms a short monitor for Steam taking foreground input. If a minimized Steam window takes focus, StageBox tries to restore launcher focus. A visible Steam prompt, including a controller firmware notice, gets pointer controls instead: left stick/D-pad moves the pointer, A clicks and B sends Escape. An instruction hint appears behind the prompt. Release the controls to neutral before using them after handoff.

The helper stops when the prompt loses focus and restores launcher input when returning. It does not automatically dismiss visible prompts or run firmware updates. Active native games, Steam/RetroBat sessions, controller tests and Bluetooth pairing keep their own input handling. This recovery uses SDL Guide detection and needs verification with your Steam version; external firmware updater applications are outside its Steam-only scope.

RetroBat's emulator-install confirmation can currently require keyboard input with a native DirectInput pad; this remains a low-priority pending issue.

## How To Use

1. Start StageBox.
2. Wait for the game grid to load local games and installed Steam games.
3. Navigate with a controller D-pad/left stick or keyboard arrows.
4. Tap the controller `A` button or press `Enter` to launch the selected game. Hold A on a native card for its input options.
5. StageBox shows a cinematic launch panel with the game's cached artwork, a green glow, and “Starting game…” until a visible game or intermediate launcher window appears, then hides and restores itself after the tracked session exits. Missing artwork uses an atmospheric moon-and-forest backdrop. Steam launches also show connection/update progress, with percentages only when available. Cursor settings, controller mapping targets, and delayed-key timing take effect at the window handoff.
6. Open the tools menu for Controllers settings/testing, Bluetooth pairing/device management, Wi-Fi networks, reloading games, opening the games folder, restart, power off, or exiting the launcher.
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
- `make help` lists build prerequisites, the Fedora SDK install command and SDK-path override.
- `make virtual-helper-deps` checks Python 3.11+ and the selected .NET SDK 10+ without downloading or publishing anything.
- `make build-linux` builds a Linux Wails binary for local development checks.
- `make virtual-helper` publishes the bundled Windows x64 helper and records its integrity manifest.
- `make build-win` builds that helper, then cross-builds `build/bin/stagebox.exe`. Override `DOTNET` or `PYTHON` for custom tool locations.
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

The optional helper bundles the pinned [HIDMaestro v1.11.0 SDK](https://github.com/hifihedgehog/HIDMaestro/releases/tag/v1.11.0), its verbatim MIT license and third-party notices, plus the self-contained .NET runtime's notices. Provenance, build steps and lifecycle details are recorded in `tools/hidmaestro/README.md`.

The official HidHide 1.5.230 installer is distributed unmodified with its MIT license, retained at `tools/hidmaestro/LICENSE-HidHide.txt` and copied into the portable helper folder. Its pinned checksum matches [Microsoft's package manifest](https://github.com/microsoft/winget-pkgs/blob/master/manifests/n/Nefarius/HidHide/1.5.230/Nefarius.HidHide.installer.yaml).
