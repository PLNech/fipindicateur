GO ?= go
BIN := fipindicateur
PKG := ./cmd/fipindicateur

# Stamp the build with `git describe` so the tray menu (and stats report)
# reflect the exact commit; each rebuild is visible on relaunch. Falls back to
# a dev placeholder outside a git checkout.
VERSION ?= $(shell git describe --tags --dirty --always 2>/dev/null || echo v0.2.0-dev)
LDFLAGS := -X github.com/PLNech/fipindicateur/internal/version.Version=$(VERSION)

# Shipping flags, for build/install/windows (not `make run`, where a symbol
# table and DWARF keep debuggers and stack traces friendly):
#   -trimpath  drops the builder's absolute paths from the binary (smaller
#              pclntab, reproducible across machines)
#   -s -w      omit the symbol table and the DWARF debug info (about a quarter
#              of the file); panics still print full Go stack traces
RELFLAGS := -trimpath -ldflags "-s -w $(LDFLAGS)"

# User-level install layout (no sudo).
PREFIX  ?= $(HOME)/.local
BINDIR  := $(PREFIX)/bin
APPDIR  := $(PREFIX)/share/applications
ICONDIR := $(PREFIX)/share/icons/hicolor
MANDIR  := $(PREFIX)/share/man/man1

.PHONY: build run test lint fix icons clean install uninstall windows web app swiftui swiftui-run swiftui-test

# Rebuild the stats report template from the web/ SPA toolchain (npm + esbuild
# + D3, subsetted WOFF2 fonts inlined). The GENERATED internal/stats/
# report.html.tmpl is committed, so `make build`/`go build` never need Node:
# run this only when the report design or its data contract changes.
WEBDIR := web
web: ## Rebuild internal/stats/report.html.tmpl from web/ (needs Node)
	cd $(WEBDIR) && npm install && npm run fonts && npm run build
	@echo "Regenerated internal/stats/report.html.tmpl. Commit it (no Node at build time)."

build: ## Build the binary
	$(GO) build $(RELFLAGS) -o $(BIN) $(PKG)

# Windows cross-build. Prerequisite (one-time, network): drop a libmpv dev
# archive into build/windows/mpv-dev so it contains include/mpv/client.h and
# libmpv.dll.a. Get it from the shinchiro builds (a mpv-dev-x86_64-*.7z from
# github.com/shinchiro/mpv-winbuild-cmake/releases) and extract:
#   mkdir -p build/windows/mpv-dev
#   7z x mpv-dev-x86_64-*.7z -obuild/windows/mpv-dev
# internal/player/cgo_windows.go points cgo at that path via ${SRCDIR}. Needs
# the mingw-w64 cross compiler (x86_64-w64-mingw32-gcc). Output and the runtime
# libmpv-2.dll land in build/windows/ next to each other.
WINDIR   := build/windows
MPVDEV   := $(WINDIR)/mpv-dev
WINEXE   := $(WINDIR)/fipindicateur.exe
windows: ## Cross-build the Windows tray binary (needs build/windows/mpv-dev, see comment)
	@test -f $(MPVDEV)/include/mpv/client.h || { echo "missing $(MPVDEV)/include/mpv/client.h; see the Makefile 'windows' comment for the libmpv dev drop"; exit 1; }
	CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc \
		$(GO) build -trimpath -ldflags "-s -w $(LDFLAGS) -H windowsgui" -o $(WINEXE) $(PKG)
	cp $(MPVDEV)/libmpv-2.dll $(WINDIR)/libmpv-2.dll
	@echo "Built $(WINEXE) (ship it alongside $(WINDIR)/libmpv-2.dll)."

run: ## Build and run
	$(GO) run -ldflags "$(LDFLAGS)" $(PKG)

# The SwiftUI popover (macOS 14+, opt-in behind the swiftui build tag; the
# default build never needs a Swift toolchain). Command Line Tools are enough,
# no Xcode. The Swift sources compile to one object dropped next to the cgo
# seam as a .syso, which Go links with the package and hashes in its build
# cache.
#   -wmo    whole-module: one object for all the sources, no archive to unpack
#   ld -r   Go refuses linker inputs carrying an __LLVM segment (it reads
#           them as LTO bitcode) and swiftc stamps its object with
#           __LLVM,__swift_modhash, a 16-byte module hash. The relocatable
#           re-link moves that section out of __LLVM; nothing else changes.
#   SWIFTLD the Swift compatibility shims live in the toolchain; the runtime
#           itself ships with macOS, so nothing is bundled.
FIPUI     := internal/popover/fipui
SWIFTARCH := $(shell uname -m)
SWIFTSYSO := $(FIPUI)/fipui_darwin_$(SWIFTARCH:x86_64=amd64).syso
SWIFTOBJ  := build/fipui.o
SWIFTLD    = -L$(shell dirname "$$(dirname "$$(xcrun -f swiftc)")")/lib/swift/macosx -L/usr/lib/swift

$(SWIFTSYSO): $(wildcard $(FIPUI)/swift/*.swift)
	@if [ "$$(uname)" != "Darwin" ]; then echo "swiftui: macOS only"; exit 1; fi
	@mkdir -p build
	swiftc -parse-as-library -emit-object -wmo -O -module-name FipUI \
		-target $(SWIFTARCH)-apple-macos14 $(FIPUI)/swift/*.swift -o $(SWIFTOBJ)
	ld -r -arch $(SWIFTARCH) -rename_section __LLVM __swift_modhash __DATA __swift_modhash \
		$(SWIFTOBJ) -o $@

swiftui: $(SWIFTSYSO) ## Build with the SwiftUI popover (macOS 14+, needs swiftc)
	CGO_LDFLAGS="$(SWIFTLD)" $(GO) build -tags swiftui $(RELFLAGS) -o $(BIN) $(PKG)

swiftui-run: $(SWIFTSYSO) ## Build and run with the SwiftUI popover
	CGO_LDFLAGS="$(SWIFTLD)" $(GO) run -tags swiftui -ldflags "$(LDFLAGS)" $(PKG)

swiftui-test: $(SWIFTSYSO) ## Vet and test the tagged build (links the Swift object)
	CGO_LDFLAGS="$(SWIFTLD)" $(GO) vet -tags swiftui ./...
	CGO_LDFLAGS="$(SWIFTLD)" $(GO) test -tags swiftui ./internal/popover/... ./internal/ui/ ./internal/tray/

test: ## Run tests
	$(GO) test ./...

lint: ## Same checks CI runs: formatting, vet, tests, build, no em dashes
	@test -z "$$(gofmt -l . | grep -v '/gen/')" || { echo "gofmt needed:"; gofmt -l . | grep -v '/gen/'; exit 1; }
	@! git grep -nIF "$$(printf '\342\200\224')" || { echo "em dash (U+2014) found, replace it (house style: middot, colon, parentheses)"; exit 1; }
	$(GO) vet ./...
	$(GO) test ./...
	$(GO) build ./...

fix: ## Auto-format and tidy
	gofmt -w .
	$(GO) mod tidy

icons: ## Regenerate the tray icons
	$(GO) run internal/icon/gen/main.go

# macOS app bundle so Spotlight, Launchpad and `open -a` can start it. A bare
# binary in ~/.local/bin is invisible to all three. The bundle is a menu-bar
# agent (LSUIElement): no Dock icon. The executable copied in is the same
# build `make install` just wrote. It has to be that Mach-O, not a shell
# script that execs it: a script makes the process a background task, and
# the status item is then placed off the menu bar.
APPSDIR ?= /Applications
APPBUND := $(APPSDIR)/FipIndicateur.app

app: install ## Build the macOS app bundle (Spotlight, Launchpad, open -a)
	@if [ "$$(uname)" != "Darwin" ]; then echo "app: macOS only"; exit 1; fi
	mkdir -p "$(APPBUND)/Contents/MacOS" "$(APPBUND)/Contents/Resources"
	sed 's|@VERSION@|$(VERSION)|g' packaging/darwin/Info.plist > "$(APPBUND)/Contents/Info.plist"
	# The bundle executable must be the Mach-O itself. A shell wrapper makes
	# Launch Services treat the process as a background task, and macOS then
	# parks the status item off the menu bar (origin y past the display).
	# Homebrew's lib dir is on the default dyld fallback path, so libmpv
	# resolves without a PATH prefix.
	cp "$(BINDIR)/$(BIN)" "$(APPBUND)/Contents/MacOS/fipindicateur"
	chmod 755 "$(APPBUND)/Contents/MacOS/fipindicateur"
	# icns from the 128px app icon. sips plus iconutil, both stock macOS tools.
	rm -rf /tmp/fipindicateur.iconset
	mkdir -p /tmp/fipindicateur.iconset
	sips -z 128 128 internal/icon/icon_app_128.png --out /tmp/fipindicateur.iconset/icon_128x128.png >/dev/null
	sips -z 256 256 internal/icon/icon_app_128.png --out /tmp/fipindicateur.iconset/icon_128x128@2x.png >/dev/null
	iconutil -c icns /tmp/fipindicateur.iconset -o "$(APPBUND)/Contents/Resources/fipindicateur.icns"
	rm -rf /tmp/fipindicateur.iconset
	cp packaging/darwin/LISEZ-MOI.txt "$(APPBUND)/Contents/Resources/LISEZ-MOI.txt"
	codesign --force --sign - "$(APPBUND)"
	# A brand-new bundle is invisible to `open -a` until Launch Services sees it.
	-/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister -f "$(APPBUND)"
	@echo "App: $(APPBUND)  (Spotlight: FipIndicateur, or: open -a FipIndicateur)"

install: ## Build and install for the current user (binary, launcher, icons, man page)
	mkdir -p $(BINDIR) $(APPDIR) $(ICONDIR)/22x22/apps $(ICONDIR)/44x44/apps $(ICONDIR)/128x128/apps $(MANDIR)
	$(GO) build $(RELFLAGS) -o $(BINDIR)/$(BIN) $(PKG)
	ln -sf $(BIN) $(BINDIR)/fip
	install -m 644 packaging/fip.1 $(MANDIR)/fip.1
	printf '.so man1/fip.1\n' > $(MANDIR)/fipindicateur.1
	sed "s|@BINDIR@|$(BINDIR)|" packaging/fipindicateur.desktop.in > $(APPDIR)/fipindicateur.desktop
	install -m 644 internal/icon/icon_app_22.png  $(ICONDIR)/22x22/apps/fipindicateur.png
	install -m 644 internal/icon/icon_app_44.png  $(ICONDIR)/44x44/apps/fipindicateur.png
	install -m 644 internal/icon/icon_app_128.png $(ICONDIR)/128x128/apps/fipindicateur.png
	-update-desktop-database $(APPDIR) 2>/dev/null || true
	-gtk-update-icon-cache -q $(ICONDIR) 2>/dev/null || true
	touch $(ICONDIR)
	install -m 755 packaging/fipindicateur-watchdog.sh $(BINDIR)/fipindicateur-watchdog
	mkdir -p $(HOME)/.config/systemd/user
	install -m 644 packaging/fipindicateur-watchdog.service $(HOME)/.config/systemd/user/fipindicateur-watchdog.service
	-systemctl --user daemon-reload 2>/dev/null || true
	-systemctl --user enable --now fipindicateur-watchdog.service 2>/dev/null || true
	@echo "Installed. Launch from GNOME activities (FipIndicateur) or $(BINDIR)/$(BIN)."

uninstall: ## Remove the user-level install (binary, launcher, icons, autostart)
	-systemctl --user disable --now fipindicateur-watchdog.service 2>/dev/null || true
	rm -f $(BINDIR)/fipindicateur-watchdog
	rm -f $(HOME)/.config/systemd/user/fipindicateur-watchdog.service
	rm -f $(BINDIR)/$(BIN)
	rm -f $(BINDIR)/fip
	rm -f $(MANDIR)/fip.1
	rm -f $(MANDIR)/fipindicateur.1
	rm -f $(APPDIR)/fipindicateur.desktop
	rm -f $(ICONDIR)/22x22/apps/fipindicateur.png
	rm -f $(ICONDIR)/44x44/apps/fipindicateur.png
	rm -f $(ICONDIR)/128x128/apps/fipindicateur.png
	rm -f $(HOME)/.config/autostart/fipindicateur.desktop
	rm -rf "$(APPBUND)"
	-update-desktop-database $(APPDIR) 2>/dev/null || true
	touch $(ICONDIR) 2>/dev/null || true

clean:
	rm -f $(BIN)
