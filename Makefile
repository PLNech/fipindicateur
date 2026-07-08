GO ?= go
BIN := fipindicateur
PKG := ./cmd/fipindicateur

# Stamp the build with `git describe` so the tray menu (and stats report)
# reflect the exact commit; each rebuild is visible on relaunch. Falls back to
# a dev placeholder outside a git checkout.
VERSION ?= $(shell git describe --tags --dirty --always 2>/dev/null || echo v0.2.0-dev)
LDFLAGS := -X github.com/PLNech/fipindicateur/internal/version.Version=$(VERSION)

# User-level install layout (no sudo).
PREFIX  ?= $(HOME)/.local
BINDIR  := $(PREFIX)/bin
APPDIR  := $(PREFIX)/share/applications
ICONDIR := $(PREFIX)/share/icons/hicolor

.PHONY: build run test lint fix icons clean install uninstall

build: ## Build the binary
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BIN) $(PKG)

run: ## Build and run
	$(GO) run -ldflags "$(LDFLAGS)" $(PKG)

test: ## Run tests
	$(GO) test ./...

lint: ## Same checks CI runs: formatting, vet, tests, build, no em dashes
	@test -z "$$(gofmt -l . | grep -v '/gen/')" || { echo "gofmt needed:"; gofmt -l . | grep -v '/gen/'; exit 1; }
	@! grep -rIn --exclude-dir=.git "$$(printf '\342\200\224')" . || { echo "em dash (U+2014) found, replace it (house style: middot, colon, parentheses)"; exit 1; }
	$(GO) vet ./...
	$(GO) test ./...
	$(GO) build ./...

fix: ## Auto-format and tidy
	gofmt -w .
	$(GO) mod tidy

icons: ## Regenerate the tray icons
	$(GO) run internal/icon/gen/main.go

install: ## Build and install for the current user (binary, launcher, icons)
	mkdir -p $(BINDIR) $(APPDIR) $(ICONDIR)/22x22/apps $(ICONDIR)/44x44/apps $(ICONDIR)/128x128/apps
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BINDIR)/$(BIN) $(PKG)
	sed "s|@BINDIR@|$(BINDIR)|" packaging/fipindicateur.desktop.in > $(APPDIR)/fipindicateur.desktop
	install -m 644 internal/icon/icon_app_22.png  $(ICONDIR)/22x22/apps/fipindicateur.png
	install -m 644 internal/icon/icon_app_44.png  $(ICONDIR)/44x44/apps/fipindicateur.png
	install -m 644 internal/icon/icon_app_128.png $(ICONDIR)/128x128/apps/fipindicateur.png
	-update-desktop-database $(APPDIR) 2>/dev/null || true
	-gtk-update-icon-cache -q $(ICONDIR) 2>/dev/null || true
	touch $(ICONDIR)
	@echo "Installed. Launch from GNOME activities (FipIndicateur) or $(BINDIR)/$(BIN)."

uninstall: ## Remove the user-level install (binary, launcher, icons, autostart)
	rm -f $(BINDIR)/$(BIN)
	rm -f $(APPDIR)/fipindicateur.desktop
	rm -f $(ICONDIR)/22x22/apps/fipindicateur.png
	rm -f $(ICONDIR)/44x44/apps/fipindicateur.png
	rm -f $(ICONDIR)/128x128/apps/fipindicateur.png
	rm -f $(HOME)/.config/autostart/fipindicateur.desktop
	-update-desktop-database $(APPDIR) 2>/dev/null || true
	touch $(ICONDIR) 2>/dev/null || true

clean:
	rm -f $(BIN)
