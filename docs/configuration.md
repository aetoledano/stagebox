# Configuration reference

[Documentation](README.md) · [StageBox](../README.md)

To run without installing the Windows shell, place `stagebox.exe` in a folder and keep the matching `assets/hidmaestro/` folder beside it for virtual input. Copy `config.example.json` to `config.json` and edit it, for example:

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

StageBox refreshes the community SDL controller database in the background on every Windows startup. `downloadControllerDatabase` defaults to `true`; set it to `false` to use the cached database without an internet request. Restart after changing this option. Keep manual edits in the [controller overrides file](features/controllers.md#controller-database-and-manual-overrides).

The config is loaded at startup and unknown JSON fields are rejected. Restart StageBox after changing it. `debug` does not reduce or disable logging. See [installation](installation.md) for the portable and kiosk setup paths.
