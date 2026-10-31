# Build on Linux, validate on Windows

[Documentation](README.md) · [StageBox](../README.md)

StageBox is developed and built on **Linux**. The release output is a Windows
x64 application, copied to Windows for real controller, game-session, driver,
Bluetooth, audio and kiosk validation. A Windows development toolchain is not
part of the normal build process.

## Required tools on the Linux build machine

Use the upstream installation guides below for your distribution. This guide
lists StageBox's requirements rather than maintaining package-manager recipes.

| Requirement | StageBox uses it for | Installation guide |
| --- | --- | --- |
| Git | Clone and manage the source | [Git on Linux](https://git-scm.com/install/linux) |
| GNU Make | Run the project build and release targets | [GNU Make](https://www.gnu.org/software/make/) |
| Go 1.27 (preferred toolchain **1.27.3**, declared in `go.mod`) | Compile the backend and install/use the Wails CLI | [Go installation](https://go.dev/doc/install) |
| Node.js 22.12+ and npm | Install/build frontend assets invoked by Wails | [Node.js downloads and installation](https://nodejs.org/en/download) |
| Wails CLI **v2.17.0** | Build the Go/WebView desktop application and generate bindings | [Wails v2 installation](https://v2.wails.io/docs/gettingstarted/installation/) |
| MinGW-w64 C and C++ cross-compilers | Compile Windows CGO/C++ integrations from Linux | [MinGW-w64 toolchains](https://www.mingw-w64.org/downloads/), [Fedora guide](https://www.mingw-w64.org/getting-started/fedora/) |
| Python 3.11+ | Prepare pinned helper dependencies and package notices | [Python on Unix](https://docs.python.org/3/using/unix.html) |
| .NET **SDK 10+** | Publish the self-contained Windows virtual-controller helper | [.NET on Linux](https://learn.microsoft.com/en-us/dotnet/core/install/linux) |
| Linux Wails dependencies | Support the Wails toolchain and local Linux development view | [Wails Linux prerequisites](https://v2.wails.io/docs/gettingstarted/installation/#platform-specific-dependencies) |

Keep the Wails CLI matched to the v2.17.0 dependency in `go.mod`. Upstream
installation guides may suggest the latest version; StageBox has not migrated
to Wails v3. The .NET **SDK** is required on Linux; the runtime alone cannot
compile the helper. Windows users receive its self-contained runtime.
After upgrading Go, also update the separately installed Wails CLI: the old
v2.11.0 package loader fails during binding generation on Go 1.27. This migration
stays on Wails v2 and refreshes the vendored dependencies and generated bindings.

The default cross-compilers are `x86_64-w64-mingw32-gcc` and
`x86_64-w64-mingw32-g++`. The audio bridge uses C++, so a C-only toolchain is
insufficient. Follow your distribution's MinGW guide for its runtime/development
packages. Tools must be on `PATH` or selected with the overrides below.

## Build a Windows bundle

After installing the requirements, run from Linux:

```sh
git clone https://github.com/aetoledano/stagebox.git
cd stagebox
make virtual-helper-deps
make release
```

`make virtual-helper-deps` checks Python and the selected .NET SDK without
downloading helper assets. `make release` builds the helper and Windows app,
then assembles the deployable directory at `release/` and the downloadable
Windows ZIP/checksum under `build/release-assets/`. Copy the complete folder or
extract the ZIP on Windows and follow [Windows setup](installation.md).

Initial builds require internet access for the pinned HIDMaestro SDK, HidHide
installer, .NET restore and frontend packages. Downloaded helper binaries,
intermediates and output bundles stay under ignored `build/` and `release/`.
Go source dependencies are vendored in the repository.

## How Wails handles the frontend

Wails is the desktop build entry point. The frontend is vanilla JavaScript/CSS
built by Vite; npm is its dependency manager, invoked **by Wails** through
[`wails.json`](../wails.json). This matches Wails' documented
[frontend command hooks](https://v2.wails.io/docs/reference/project-config/).

| Wails hook | Configured command | Purpose |
| --- | --- | --- |
| `frontend:install` | `npm ci` | Install exact dependencies from the lockfile when Wails runs installation |
| `frontend:build` | `npm run build` | Build Vite assets into `frontend/dist` for embedding in Go |
| `frontend:dev:watcher` | `npm run dev` | Start Vite while Wails runs the app in development |

Normal full builds use `make` / Wails. There is no separate npm installation
step required before that flow. Node.js and npm are still build requirements;
Wails delegates the frontend build to them rather than replacing them.

For a frontend-only production build, explicitly run:

```sh
npm --prefix frontend ci
npm --prefix frontend run build
```

This builds assets only; it does not produce a usable Windows launcher or run
the Go backend. A standalone Vite preview lacks native Wails bindings unless a
documentation fixture supplies them. Commit `frontend/package.json` and
`frontend/package-lock.json` together when changing dependencies.

## Project commands

| Command | Result |
| --- | --- |
| `make` / `make build-win` | Build the paired helper, then `build/bin/stagebox.exe` |
| `make release` | Recreate `release/`, then generate the Windows ZIP and checksum in `build/release-assets/` |
| `make virtual-helper-deps` | Check Python/.NET helper prerequisites |
| `make virtual-helper` | Publish `build/bin/assets/hidmaestro/` and refresh the embedded integrity manifest |
| `make help` | Show build prerequisites and supported tool-path overrides |
| `make dev` | Run the Linux Wails development view, using the `webkit2_41` tag |
| `make build-linux` | Build a Linux development binary with platform stubs |
| `make dep` | Intentionally run Go dependency maintenance and regenerate `vendor/` |

Linux development views are useful for frontend work; they do not establish
that Windows platform behavior works. The runtime reads local `config.json`;
copy `config.example.json` and edit its path if running a development view.

Supported Make overrides include `APP_VERSION`, `DOTNET`, `PYTHON`, `MINGW`, `WIN_CC`, `WIN_CXX`,
`APP_ICON`, `WAILS_TAGS`, `GO_BUILD_CACHE`, `RELEASE_DIR`, `RELEASE_ASSETS_DIR` and
`SOURCE_COMMIT`. For example,
`make DOTNET=/absolute/path/to/dotnet` selects an existing SDK. Do not depend on
temporary SDK paths from another developer's session.

## Release integrity and Windows validation

Rebuild the Go executable whenever the helper manifest changes. Always deploy
the matching `assets/hidmaestro/` directory with it; mixing builds fails
validation. See the [helper provenance/build guide](../tools/hidmaestro/README.md)
and [publishing guide](publishing.md).

Tests are maintainer-run. For controller/session changes, ask the maintainer to
run the relevant tests on Windows, commonly:

```sh
go test ./internal/platform/controller ./internal/games
```

Also check real controller input, remapping/rumble, native/Steam/RetroBat session
return, Bluetooth and any affected kiosk behavior on Windows. Record builds,
tests and hardware checks separately; do not present a cross-build as a Windows
runtime check.

## Source conventions

Read [CODEX.md](../CODEX.md) for architecture, data flow and platform traps.
Keep Windows build tags and non-Windows stubs, keep generated Wails bindings
aligned with bound Go methods, and avoid unrelated vendor edits. Update user
guides for visible behavior and CODEX for architecture/build changes. Keep
local config, device profiles, logs and recovery journals out of commits.

## Automatic release bundles

`.github/workflows/release.yml` runs the Linux cross-build when a GitHub release
is published from a tag whose commit belongs to `main`. It pins Go from `go.mod`,
Node 22, .NET SDK 10, and Wails v2.17.0; drafts and ordinary pushes do not trigger
it. A manual **Run workflow** on `main` verifies the build and saves an Actions
artifact without creating or uploading to a release. See [publishing](publishing.md)
for release steps, manual verification and recovery guidance.

`make release APP_VERSION=v0.43` embeds that version in the Windows app's startup
log without editing `main.go` and creates `stagebox-v0.43-windows-x64.zip` plus
its `.zip.sha256` file. Plain `make release` uses the version declared in
`main.go`, currently `0.42`. It also writes `BUILD-INFO.json` into the staged
folder and ZIP. No separate packaging command is required locally or in CI.

The packaging script can also be invoked directly for an already-built,
matching staging directory:

```sh
python3 tools/package_release.py --version v0.43 --source-commit "$(git rev-parse HEAD)"
```

It validates the release files and every helper manifest hash, rejects local
runtime config/logs, and creates the ZIP/checksum. Use the same version for the
build and any manual repackaging. `SOURCE_COMMIT` defaults to Git HEAD; supply
the exact commit explicitly when building from a source archive without `.git`.
Neither `make release` nor the workflow runs tests.
