APP ?= stagebox
MINGW  ?= x86_64-w64-mingw32
WIN_CC ?= $(MINGW)-gcc
WIN_CXX?= $(MINGW)-g++
WAILS_TAGS ?= webkit2_41
RELEASE_DIR ?= release
GO_BUILD_CACHE ?= /tmp/launcher-go-build

default: build-win

.PHONY: dev
dev:
	@wails dev -tags '$(WAILS_TAGS)'

.PHONY: build-linux
build-linux:
	@wails build -tags '$(WAILS_TAGS)'

.PHONY: build-win
build-win:
	@echo "→ Building Windows amd64 .exe with Wails…"
	@GOCACHE=$(GO_BUILD_CACHE) CGO_ENABLED=1 CC=$(WIN_CC) CXX=$(WIN_CXX) GOOS=windows GOARCH=amd64 \
		wails build -platform windows/amd64
	@echo "→ Output:"
	@ls -lh build/bin/stagebox.exe 2>/dev/null || true

.PHONY: release sbrelease
release: build-win
	@echo "→ Creating StageBox release bundle in $(RELEASE_DIR)…"
	@rm -rf "$(RELEASE_DIR)"
	@mkdir -p "$(RELEASE_DIR)"
	@cp build/bin/stagebox.exe "$(RELEASE_DIR)/stagebox.exe"
	@cp custom_scripts/install-stagebox.ps1 "$(RELEASE_DIR)/install-stagebox.ps1"
	@cp custom_scripts/svc.vbs "$(RELEASE_DIR)/svc.vbs"
	@cp custom_scripts/systemsvcs.ps1 "$(RELEASE_DIR)/systemsvcs.ps1"
	@cp custom_scripts/elevated-PS.cmd "$(RELEASE_DIR)/elevated-PS.cmd"
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
# sudo dnf install gcc gcc-c++ pkgconf-pkg-config gtk3-devel webkit2gtk4.1-devel libsoup3-devel mingw64-winpthreads mingw64-zlib mingw64-gcc mingw64-gcc-c++ mingw64-crt mingw64-headers
