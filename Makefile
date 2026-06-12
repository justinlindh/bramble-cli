BINARY ?= bramble
PKG ?= ./cmd/bramble
VERSION_FILE ?= VERSION

VERSION ?= $(shell \
	if [ -f $(VERSION_FILE) ]; then \
		tr -d '\n' < $(VERSION_FILE); \
	else \
		git describe --tags --dirty 2>/dev/null || echo dev; \
	fi)

LDFLAGS = -X github.com/justinlindh/bramble-cli/internal/commands.version=$(VERSION)

.PHONY: build build-dev print-version

print-version:
	@echo $(VERSION)

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) $(PKG)

build-dev:
	go build -o $(BINARY) $(PKG)
