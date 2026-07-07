GO ?= go
BIN := fipindicateur
PKG := ./cmd/fipindicateur

.PHONY: build run test lint fix icons clean

build: ## Build the binary
	$(GO) build -o $(BIN) $(PKG)

run: ## Build and run
	$(GO) run $(PKG)

test: ## Run tests
	$(GO) test ./...

lint: ## Same checks CI runs: formatting, vet, tests, build
	@test -z "$$(gofmt -l . | grep -v '/gen/')" || { echo "gofmt needed:"; gofmt -l . | grep -v '/gen/'; exit 1; }
	$(GO) vet ./...
	$(GO) test ./...
	$(GO) build ./...

fix: ## Auto-format and tidy
	gofmt -w .
	$(GO) mod tidy

icons: ## Regenerate the tray icons
	$(GO) run internal/icon/gen/main.go

clean:
	rm -f $(BIN)
