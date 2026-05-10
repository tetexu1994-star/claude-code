.PHONY: all build build-all clean test vet lint fmt race install uninstall dist release help

APP_NAME    := tlaude-code
CMD_DIR     := ./cmd/$(APP_NAME)
BUILD_DIR   := .
GO          := go
GOFLAGS     :=
VERSION     ?= dev
COMMIT      ?= unknown
DATE        ?= unknown
LDFLAGS     := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

all: build

build:
	$(GO) build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME) $(CMD_DIR)

build-all:
	@echo "Building for all platforms..."
	GOOS=darwin  GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME)-darwin-amd64 $(CMD_DIR)
	GOOS=darwin  GOARCH=arm64 $(GO) build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME)-darwin-arm64 $(CMD_DIR)
	GOOS=linux   GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME)-linux-amd64 $(CMD_DIR)
	GOOS=linux   GOARCH=arm64 $(GO) build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME)-linux-arm64 $(CMD_DIR)
	@echo "Done. Binaries in $(BUILD_DIR)/"

dist:
	@test -n "$$(command -v goreleaser 2>/dev/null)" || { echo "goreleaser not found. Install: brew install goreleaser"; exit 1; }
	goreleaser release --clean --snapshot --skip=publish

release:
	@test -n "$(VERSION)" || { echo "Usage: make release VERSION=1.0.0"; exit 1; }
	@git diff --quiet || { echo "Uncommitted changes. Commit or stash first."; exit 1; }
	git tag -a "v$(VERSION)" -m "Release v$(VERSION)"
	git push origin "v$(VERSION)"

install:
	$(GO) install $(GOFLAGS) -ldflags="$(LDFLAGS)" $(CMD_DIR)

uninstall:
	rm -f "$(shell go env GOPATH)/bin/$(APP_NAME)"

test:
	$(GO) test -count=1 ./...

vet:
	$(GO) vet ./...

race:
	$(GO) test -race -count=1 ./...

lint:
	golangci-lint run ./...

fmt:
	$(GO) fmt ./...

clean:
	rm -f $(BUILD_DIR)/$(APP_NAME) $(BUILD_DIR)/$(APP_NAME)-*
	rm -rf $(BUILD_DIR)/dist/
	rm -rf /tmp/tlaude-code-*

tidy:
	$(GO) mod tidy

help:
	@echo "Usage:"
	@echo "  make build        - Build for current platform"
	@echo "  make build-all    - Cross-compile for all platforms (darwin/linux, amd64/arm64)"
	@echo "  make install      - Install to GOPATH/bin"
	@echo "  make uninstall    - Remove from GOPATH/bin"
	@echo "  make dist         - Build snapshot with GoReleaser (local)"
	@echo "  make release      - Tag and push a release (e.g. make release VERSION=1.0.0)"
	@echo "  make test         - Run all tests"
	@echo "  make vet          - Run go vet"
	@echo "  make race         - Run tests with race detector"
	@echo "  make lint         - Run golangci-lint (if installed)"
	@echo "  make fmt          - Format all Go code"
	@echo "  make clean        - Remove build artifacts"
	@echo "  make tidy         - Tidy go.mod"
