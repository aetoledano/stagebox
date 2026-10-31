APP ?= stagebox
APP_ICON ?= assets/app-icon.png
MINGW  ?= x86_64-w64-mingw32
WIN_CC ?= $(MINGW)-gcc
WIN_CXX?= $(MINGW)-g++
WAILS_TAGS ?= webkit2_41
RELEASE_DIR ?= release
GO_BUILD_CACHE ?= /tmp/launcher-go-build
DOTNET ?= dotnet
PYTHON ?= python3

default: build-win

.PHONY: help
help:
	@echo "StageBox Windows build: make (or make build-win); staging bundle: make release"
	@echo "Build tools: Go, Node/npm, Wails v2, MinGW-w64, Python 3.11+, .NET SDK 10+"
	@echo "Check helper prerequisites: make virtual-helper-deps"
	@echo "Fedora .NET SDK installation: sudo dnf install dotnet-sdk-10.0"
	@echo "Use an existing SDK: make DOTNET=/absolute/path/to/dotnet"
	@echo "Windows runtime users do not need to install .NET; it is bundled."

.PHONY: dev
dev:
	@wails dev -tags '$(WAILS_TAGS)'

.PHONY: build-linux
build-linux: app-icon
	@wails build -tags '$(WAILS_TAGS)'

.PHONY: app-icon
app-icon:
	@test -f "$(APP_ICON)" || { echo "Missing app icon: $(APP_ICON)"; exit 1; }
	@mkdir -p build
	@cp "$(APP_ICON)" build/appicon.png

.PHONY: build-win
build-win: app-icon virtual-helper
	@echo "→ Building Windows amd64 .exe with Wails…"
	@GOCACHE=$(GO_BUILD_CACHE) CGO_ENABLED=1 CC=$(WIN_CC) CXX=$(WIN_CXX) GOOS=windows GOARCH=amd64 \
		wails build -platform windows/amd64
	@echo "→ Output:"
	@ls -lh build/bin/stagebox.exe 2>/dev/null || true

.PHONY: virtual-helper
virtual-helper: virtual-helper-deps
	@"$(PYTHON)" tools/hidmaestro/prepare.py
	@DOTNET_CLI_TELEMETRY_OPTOUT=1 "$(DOTNET)" publish tools/hidmaestro/StageBox.HIDMaestro.csproj -c Release -o build/bin/assets/hidmaestro
	@"$(PYTHON)" tools/hidmaestro/prepare.py manifest
	@cp custom_scripts/install-hidhide.ps1 build/bin/install-hidhide.ps1

.PHONY: virtual-helper-deps
virtual-helper-deps:
	@command -v "$(PYTHON)" >/dev/null 2>&1 || { \
		echo "Missing Python executable: $(PYTHON). Install Python 3.11+ or use PYTHON=/path/to/python."; \
		exit 1; }
	@"$(PYTHON)" -c 'import sys; sys.exit(0 if sys.version_info >= (3, 11) else "The helper build requires Python 3.11+; select it with PYTHON=/path/to/python.")'
	@command -v "$(DOTNET)" >/dev/null 2>&1 || { \
		echo "Missing .NET SDK executable: $(DOTNET). The Windows helper requires SDK 10 or newer."; \
		echo "Fedora: sudo dnf install dotnet-sdk-10.0"; \
		echo "Other systems: https://learn.microsoft.com/dotnet/core/install/"; \
		echo "Already installed elsewhere? Run make DOTNET=/absolute/path/to/dotnet"; \
		exit 1; }
	@"$(PYTHON)" -c 'import subprocess, sys; result = subprocess.run([sys.argv[1], "--version"], capture_output=True, text=True); version = result.stdout.strip(); major = version.split(".")[0]; sys.exit(0 if result.returncode == 0 and major.isdigit() and int(major) >= 10 else "The selected dotnet requires an SDK version 10 or newer, not just a runtime. Fedora: sudo dnf install dotnet-sdk-10.0; or set DOTNET=/path/to/a/suitable/dotnet.")' "$(DOTNET)"

.PHONY: release sbrelease
release: build-win
	@echo "→ Creating StageBox release bundle in $(RELEASE_DIR)…"
	@rm -rf "$(RELEASE_DIR)"
	@mkdir -p "$(RELEASE_DIR)"
	@cp build/bin/stagebox.exe "$(RELEASE_DIR)/stagebox.exe"
	@mkdir -p "$(RELEASE_DIR)/assets"
	@cp -R build/bin/assets/hidmaestro "$(RELEASE_DIR)/assets/hidmaestro"
	@cp custom_scripts/install-stagebox.ps1 "$(RELEASE_DIR)/install-stagebox.ps1"
	@cp custom_scripts/install-hidhide.ps1 "$(RELEASE_DIR)/install-hidhide.ps1"
	@cp custom_scripts/svc.vbs "$(RELEASE_DIR)/svc.vbs"
	@cp custom_scripts/systemsvcs.ps1 "$(RELEASE_DIR)/systemsvcs.ps1"
	@cp custom_scripts/elevated-PS.cmd "$(RELEASE_DIR)/elevated-PS.cmd"
	@cp config.example.json "$(RELEASE_DIR)/config.example.json"
	@"$(PYTHON)" tools/package_notices.py "$(RELEASE_DIR)"
	@echo "→ Release bundle contents:"
	@ls -lh "$(RELEASE_DIR)"

sbrelease: release

.PHONY: dep
dep:
	@echo "→ Ensuring Go dependencies are up to date…"
	@go mod tidy && go mod vendor

.PHONY: gccdeps
gccdeps:
	@echo "→ Ensuring cross-compiler $(WIN_CC) is installed…"
	@command -v $(WIN_CC) >/dev/null || { \
		echo "Missing $(WIN_CC). Install MinGW-w64:"; \
		echo "  sudo apt install gcc-mingw-w64 g++-mingw-w64"; \
		exit 1; }

# dependencies names
# C# helper SDK on Fedora: sudo dnf install dotnet-sdk-10.0
# Install the SDK, not only dotnet-runtime; make help lists the build prerequisites.
# sudo dnf install gcc gcc-c++ pkgconf-pkg-config gtk3-devel webkit2gtk4.1-devel libsoup3-devel mingw64-winpthreads mingw64-zlib mingw64-gcc mingw64-gcc-c++ mingw64-crt mingw64-headers
