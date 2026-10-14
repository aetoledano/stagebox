# StageBox HIDMaestro helper

This Windows x64 helper creates one Xbox 360 controller per converted physical pad (up to four)
for native games, Steam sessions/game cards and RetroBat, based on saved controller defaults.
The per-game switch additionally opts native cards into conversion; Steam/RetroBat use only defaults. StageBox stays unelevated; only this fixed helper uses Windows UAC.
The self-contained .NET 10 runtime lives beside it, so Windows does not need a
separate .NET installation. HIDMaestro still registers Windows driver packages
and a locally trusted certificate on first use. Creating each device requires
administrator privileges. No ViGEmBus or USB/IP profile is used.

## Build and provenance

Install Python 3.11+ and .NET SDK 10 or newer on the development machine. On Fedora:

```sh
sudo dnf install dotnet-sdk-10.0
```

The SDK is required for compilation; installing only the runtime is insufficient.
Use `make help` for prerequisites and `make virtual-helper-deps` to check them
without fetching assets. Then run:

```sh
make virtual-helper
make build-win
```

Use `make DOTNET=/path/to/dotnet` and `PYTHON=/path/to/python` for custom tool locations.
The executable paths may contain spaces. The SDK used during initial agent builds
lived in `/tmp/stagebox-dotnet` with an explicit `DOTNET` override; that temporary
installation was not added to the user's PATH and is not a project prerequisite
location. See [Microsoft's Fedora SDK instructions](https://learn.microsoft.com/en-us/dotnet/core/install/linux-fedora).
`prepare.py` downloads the official pinned [HIDMaestro v1.11.0 release](https://github.com/hifihedgehog/HIDMaestro/releases/tag/v1.11.0)
to ignored `build/HIDMaestro-v1.11.0.zip` and verifies SHA-256:

```text
51c5a957feb7573cf60ee8f30f791dd013053208eaf23d013e28b438303fbdf2
```

Only the SDK and its Windows interop dependencies are extracted into
`build/hidmaestro-sdk`; upstream tools/driver/profile resources are embedded in
the SDK. `StageBox.HIDMaestro.csproj` publishes to
`build/bin/assets/hidmaestro`, including the verbatim upstream MIT license and
third-party notices. The published directory is about 170 MiB including HidHide. The Go backend
embeds `internal/platform/controller/hidmaestro_manifest.json`, generated after
publish, and verifies every managed file before elevation. Rebuild the helper
and StageBox together: mixing bundles makes integrity validation fail.

`make release` includes the entire helper directory. Keep
`assets/hidmaestro` beside `stagebox.exe` when deploying; copying only the helper
EXE is insufficient. The kiosk installer copies this managed directory without
touching controller mapping files. Build downloads happen on the development
machine; the app does not download executable helper updates at runtime.

## Input and lifecycle

The Go backend pins the ordered converted physical SDL paths before launching the helper.
It authenticates a localhost TCP connection with a random per-session token.
The helper verifies the exact parent PID/creation time, accepts a bounded
authenticated setup request with one to four unique physical HID paths, then receives
fixed ordered N × 12-byte little-endian XInput report batches, with no command-execution protocol.
Standard face/shoulder/stick buttons, D-pad, Start/Back, both sticks and both
triggers are mapped into the SDK's `xbox-360-wired` profile. Y axes invert to HID
coordinates. Setup must expose one new XInput slot per converted pad before the game or frontend is started.

The SDL thread publishes the latest state without waiting for IPC. A separate
worker sends it while launcher navigation is paused, with a 100 ms heartbeat
and neutral state after 200 ms without SDL samples. The helper releases its
controllers on EOF, parent exit or 1.5 seconds without reports. Only its own
controllers are disposed, with `HMContext.Dispose` doing parallel teardown. `InstallDriver()` is called only when the matching
driver is absent, because that SDK method includes a global stale-device sweep.

Each converted pad has independent state and neutral rearming. Native-default pads
remain physical unless the game switch is on; Virtual-default pads always convert.
Native XInput stays native. All sources reconnect/rearm independently. Rumble/output forwarding is
not implemented. HidHide protects the physical DirectInput controller so games
can use the virtual Xbox pad without duplicate input.
Verified SDL `XInput#` sources bypass virtual creation. Unplugging a pinned
physical device sends neutral reports; a matching device path can reconnect.
Another controller does not silently replace it during a session. New unrelated
pads are deferred until return. Connect the intended roster before launching.
There are four total XInput slots, including native pads; insufficient slots block
launch instead of silently omitting a player. `FinalizeNames` runs once after creation.

The driver/certificate installation remains on Windows after the game ends;
the per-session devices do not. See the upstream [driver and signing documentation](https://hidmaestro.org/docs/reference/driver-install-and-signing/).
Real Windows driver creation, cleanup, controls and reconnects need hardware
verification. Do not run tests without the user's instruction.

## HidHide setup and session ownership

`prepare.py` also downloads the unmodified official
[HidHide 1.5.230 installer](https://github.com/nefarius/HidHide/releases/tag/v1.5.230.0)
to ignored `build/HidHide_1.5.230_x64.exe`, copies it into the helper folder with
`LICENSE-HidHide.txt`, and verifies SHA-256 before recording the bundle manifest:

```text
f4bbbcb82e6258641b887c74bc81c4c5f66e4aa811808dfc304347687b7605f6
```

The digest and `/exenoui /qn /norestart` switches match
[Microsoft's winget manifest](https://github.com/microsoft/winget-pkgs/blob/master/manifests/n/Nefarius/HidHide/1.5.230/Nefarius.HidHide.installer.yaml).
No installer is downloaded or executed by a game launch. The standalone
`custom_scripts/install-hidhide.ps1` verifies the pinned installer and installs
the driver from an Administrator PowerShell session, preserving existing
installations and HidHide lists. The general `install-stagebox.ps1` copies and
invokes this script before shell/task setup. It checks the 64-bit package marker
and driver service, waits for installation, validates the exit code and service
registration, logs the result and requests a Windows restart after a new install.
It never restarts automatically. The script is also copied to `build/bin` and
the release root; an explicit `-InstallerPath` or custom `-LauncherDir` is supported.
The helper only checks the control device/package marker and shows instructions
to run that script if missing. Existing installations, including compatible
newer drivers, are reused. A busy/unavailable driver requires closing its
configuration client or restarting, not another installation during launch.

`HidHide.cs` uses the released `\\.\HidHide` configuration device and
IOCTL functions 2048–2055, not the newer unreleased session-blacklist API.
`CM_Get_Device_Interface_PropertyW` resolves each selected SDL HID interface to
its exact `HID\...` instance. Only these selected HID children are hidden; no wildcard,
parent/container sweep or unrelated HID node is modified. StageBox's NT image
path is temporarily allowlisted. An unallowlisted helper open must fail with
access denied, and a fresh StageBox open must succeed, before launch readiness.
The virtual Xbox controller is never added to the hide list. A game already
holding a physical handle or using Raw Input is outside this protection.

A global StageBox lease mutex prevents overlapping ownership. Before mutation,
the helper durably records its PID/creation time, selected instances, added app
and device entries and changed cloak state in `assets/hidhide-session.json`.
The version-2 journal stores arrays and recovery accepts old version-1 journals.
Cleanup re-reads and merges current configuration: it removes only additions it
owns, keeps pre-existing entries and inversion unchanged, and restores a cloak
state it enabled unless doing so would disable newly configured unrelated pads.
Setup refuses inverted application mode and enabling a previously inactive list
containing unrelated devices. Do not replace all user lists or turn off hiding
globally as a blanket recovery.

Socket EOF, input timeout and parent exit run normal lease disposal. If the
helper is itself killed or Windows reboots, the journal remains; the next
StageBox startup launches this fixed helper unelevated with `--recover-hidhide`
before SDL polling, only when a journal exists. The public configuration API
requires no administrator privileges. Recovery never clears a live PID/creation
owner. Do not delete the journal until restoration succeeds; a forced helper
termination can leave hiding active until that recovery runs.

References: [public driver API](https://docs.nefarius.at/projects/HidHide/API-Documentation/),
[released IOCTL definitions](https://github.com/nefarius/HidHide/blob/v1.5.230.0/HidHide/src/Logic.h),
and [stack/reconnect limitations](https://docs.nefarius.at/projects/HidHide/FAQ/).
