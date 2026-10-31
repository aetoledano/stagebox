# Windows setup

[Documentation](README.md) · [StageBox](../README.md)

## What you need

- A Windows 10/11 **x64** PC and your games.
- A complete Windows bundle from [StageBox Releases](https://github.com/aetoledano/stagebox/releases),
  or one you [build on Linux](development.md). Download `stagebox-<version>-windows-x64.zip`;
  GitHub's automatic Source code downloads contain source rather than the runnable app.
- The [Microsoft Edge WebView2 Runtime](https://developer.microsoft.com/en-us/microsoft-edge/webview2/).
  Use Microsoft's installation guide if it is missing.
- A controller or keyboard for the library. Keep a keyboard/mouse available for
  Windows installers, sign-in and elevation dialogs.

You do not need Go, Node.js, Python or the .NET SDK on the Windows gaming PC.
The virtual-controller helper includes its .NET runtime. Administrator access
is needed for kiosk/driver setup and virtual-device sessions.

## Quick setup: run as a regular app

1. Unpack the **whole** bundle into a folder on Windows. Keep the matching
   `assets/hidmaestro/` folder beside `stagebox.exe` for optional virtual input.
2. Copy `config.example.json` to `config.json`. Set `gamesRoot` to your games
   folder; leave `debug: true` for regular app use. See [configuration](configuration.md).
3. Run `stagebox.exe` from that folder. Fully installed Steam games are found
   automatically. For other games, follow [add your own games](game-library.md).

Navigate with your controller and tap A to launch, or use arrows and Enter.
See [your first session](getting-started.md) for the rest of the controls.

## Optional kiosk setup

Use this for a dedicated gaming PC or cabinet where StageBox replaces the usual
Windows shell. Check the regular app setup first. The installer can also offer
automatic logon; read [Security](../SECURITY.md) before enabling it on a shared PC.

Move the complete bundle to the Windows machine, open PowerShell **as
Administrator** in that folder, and run:

```powershell
.\install-stagebox.ps1
```

The installer stages StageBox under `C:\launcher`, creates a default config when
needed, installs HidHide, registers the user shell and scheduled helpers, and
prepares Wi-Fi/Bluetooth services. It preserves an existing HidHide installation.
After a new driver install, restart Windows once; the script does not restart
the machine automatically. Automatic logon is an explicit opt-in and stores the
chosen account password in the Winlogon registry.

## Optional virtual-controller driver setup

For virtual Xbox input on an existing regular-app installation, open PowerShell
as Administrator in the bundle folder and run:

```powershell
.\install-hidhide.ps1
```

Keep `HidHide_1.5.230_x64.exe` under `assets\hidmaestro` or beside the script.
The script also searches `C:\launcher\assets\hidmaestro`. Use `-LauncherDir` for
a different StageBox directory or `-InstallerPath` for an explicit installer.
It checks the pinned checksum and leaves existing HidHide device/application
lists intact. Restart after a new install before trying virtual input.

The standalone log is `hidhide-install.log` beside the script; the full kiosk
installer records setup in `installation.log`. See [virtual Xbox input](features/virtual-input.md)
for session behavior, UAC prompts and controller limits.

## Keep the bundle together

The generated `release/` directory contains `stagebox.exe`, its matching
`assets/hidmaestro/` helper, example config, installation/service scripts,
`START-HERE.txt`, `BUILD-INFO.json` in packaged ZIPs, `LICENSE`,
`THIRD_PARTY_NOTICES.md`, `licenses/`, and the artwork inventory.
Copy the directory contents together. Mixing an executable and helper from
different builds fails the helper integrity check.
