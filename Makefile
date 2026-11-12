BINARY  := nantucket
PKG     := ./cmd/$(BINARY)
GO      ?= go
GOFLAGS ?=
LDFLAGS ?= -s -w
VERSION := $(shell sed -n 's/^const Version = "\(.*\)"/\1/p' internal/vm/builtins.go)

# Release archives (make dist): set GOOS/GOARCH to cross-compile.
GOOS    ?= $(shell $(GO) env GOOS)
GOARCH  ?= $(shell $(GO) env GOARCH)
DIST    := dist/$(BINARY)-$(VERSION)-$(GOOS)-$(GOARCH)
EXE     := $(if $(filter windows,$(GOOS)),.exe,)
# Windows users expect a .zip; everyone else gets a .tar.gz.
ARCHIVE_EXT := $(if $(filter windows,$(GOOS)),.zip,.tar.gz)
ARCHIVE     := $(if $(filter windows,$(GOOS)),zip -q -r -X,tar -czf)

.DEFAULT_GOAL := all

.PHONY: all build test test-race test-v test-prg cover vet vulncheck fmt fmt-check tidy run demo example install dist dist-all clean help

all: fmt-check vet test build test-prg ## Check formatting, vet, test, build, then run the .prg tests

build: ## Build the nantucket binary
	$(GO) build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BINARY) $(PKG)

test: ## Run all tests
	$(GO) test $(GOFLAGS) ./...

test-v: ## Run all tests verbosely
	$(GO) test $(GOFLAGS) -v ./...

test-prg: build ## Run the example .prg tests with "nantucket test"
	./$(BINARY) test examples

test-race: ## Run tests with the race detector
	$(GO) test $(GOFLAGS) -race ./...

cover: ## Run tests and write coverage.html
	$(GO) test $(GOFLAGS) -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out | tail -1
	$(GO) tool cover -html=coverage.out -o coverage.html

vet: ## Run go vet
	$(GO) vet ./...

vulncheck: ## Scan dependencies for known vulnerabilities (downloads govulncheck)
	$(GO) run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...

fmt: ## Format all Go source
	gofmt -w cmd internal *.go

fmt-check: ## Fail if any Go source needs formatting
	@out=$$(gofmt -l cmd internal *.go); \
	if [ -n "$$out" ]; then echo "gofmt needed on:"; echo "$$out"; exit 1; fi

tidy: ## Tidy go.mod / go.sum
	$(GO) mod tidy

run: build ## Build and start the dot prompt
	./$(BINARY)

demo: build ## Build and run the guided tour
	./$(BINARY) examples/demo

example: build ## Build and run the contacts example
	./$(BINARY) examples/contacts

install: ## Install into GOBIN (or GOPATH/bin)
	$(GO) install $(GOFLAGS) -ldflags '$(LDFLAGS)' $(PKG)

dist: ## Build a release archive with LICENSE, NOTICE and THIRD_PARTY_LICENSES (GOOS/GOARCH to cross-compile; .zip for Windows)
	rm -rf $(DIST) $(DIST).tar.gz $(DIST).zip
	mkdir -p $(DIST)/examples
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=0 $(GO) build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(DIST)/$(BINARY)$(EXE) $(PKG)
	cp LICENSE NOTICE THIRD_PARTY_LICENSES README.md WHATSNEW.md COMMANDS.md DOCUMENTATION.md $(DIST)/
	cp examples/*.prg $(DIST)/examples/
	cd dist && $(ARCHIVE) $(notdir $(DIST))$(ARCHIVE_EXT) $(notdir $(DIST))
	@echo "Built $(DIST)$(ARCHIVE_EXT)"

dist-all: ## Build release archives for Linux, macOS and Windows, with SHA256SUMS
	rm -f dist/SHA256SUMS
	@for p in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64; do \
		$(MAKE) --no-print-directory dist GOOS=$${p%/*} GOARCH=$${p#*/} || exit 1; \
	done
	cd dist && sha256sum $(BINARY)-$(VERSION)-*.tar.gz $(BINARY)-$(VERSION)-*.zip > SHA256SUMS
	@echo "Built dist/SHA256SUMS"

clean: ## Remove build outputs
	rm -rf $(BINARY) coverage.out coverage.html dist
	$(GO) clean -testcache

help: ## List targets
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*## "}; {printf "  %-10s %s\n", $$1, $$2}'
