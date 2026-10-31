# Troubleshooting

[Documentation](README.md) · [StageBox](../README.md)

Start with the symptom below. Keep your StageBox version and a short description of what you did when [reporting an issue](https://github.com/aetoledano/stagebox/issues).

## Logs and common problems

- `stagebox.log` is written in the launcher's working directory and preserved across restarts. Its absolute path is logged at startup. At startup, logs of at least 8MiB rotate to `stagebox.log.previous`, replacing the previous backup; a single long run can grow beyond that size.
- Logging always includes DEBUG messages. `config.json`'s `debug` flag controls exit behavior, not log verbosity.
- For intermittent Steam launch failures, capture the log around `Steam game requested`, `Steam interactive game page requested`, `Steam game startup status`, and `Steam game process exited`. These show activation, update flags/counters and native exit status. Redact local paths, network names and device identifiers before sharing logs.
- `svc.log` is written by the service/helper scripts on Windows.
- If no local games appear, check that `config.json` points to a readable `gamesRoot` and that each game folder contains a valid `lc.json`. For missing Steam games, check Steam is installed, the library drive is available, and downloads have finished; reload StageBox and inspect its Steam discovery logs.
- If a game opens another launcher and StageBox returns too early, try `"trackMode": "processTree"`.
- If Wi-Fi scanning fails in kiosk mode, rerun `install-stagebox.ps1` as Administrator and inspect `svc.log`.
- If the screen is black after shell startup, inspect `stagebox.log` for frontend boot and render markers.
- For controller problems, inspect `controller backend SDL3 initialized`, `SDL3 controller mappings loaded`, `SDL3 joystick discovered`, `SDL3 gamepad mapping`, `SDL3 gamepad connected/disconnected`, and `controller input device changed` in `stagebox.log`. Joystick discovery includes name, VID/PID and mapped status, even when the device falls back to native input. SDL extraction/load/init/discovery errors log the fallback reason. Check the cache is writable, try the legacy backend, and verify disconnect/reconnect plus game return on the target Windows machine.

## “Virtual controller file changed” after an update

The launcher checks every bundled helper file against hashes embedded in
`stagebox.exe`. A mismatch blocks virtual-input game launches before the helper
starts. The helper folder is resolved beside the **running executable**, even
when a shortcut uses a different working directory.

1. Exit games and StageBox completely before replacing any files.
2. Extract the complete Windows ZIP into a new, empty folder. Run its
   `stagebox.exe` directly so an older shortcut cannot select another copy.
3. Bring over your existing configuration/controller settings if needed, keeping
   the new ZIP's executable and complete `assets/hidmaestro/` folder together.
   Avoid merging helper files from different builds or updating a running copy.

If a fresh extraction still fails, record the exact DLL name, the ZIP's
`BUILD-INFO.json`, and the SHA-256 values of `stagebox.exe` and the reported DLL.
For example, run PowerShell from the extracted folder:

```powershell
Get-FileHash .\stagebox.exe, .\assets\hidmaestro\System.Reflection.Emit.Lightweight.dll -Algorithm SHA256
```

## Controller and launch guides

- [Controller defaults, mapping and diagnostics](features/controllers.md)
- [Virtual Xbox setup, driver requirements and limits](features/virtual-input.md)
- [Bluetooth pairings](features/bluetooth.md)
- [Steam discovery, updates and launch tracking](features/steam.md)
- [RetroBat discovery and session return](features/retrobat.md)

Avoid posting full logs before reviewing them for local paths, network names and device identifiers. Security-sensitive reports belong in the [security reporting process](../SECURITY.md).
