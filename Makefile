# Default goal
.DEFAULT_GOAL := help

GO ?= go
PORT ?= 8443
IFACE ?= 0.0.0.0
TARGET ?= 127.0.0.1:8443
RETRIES ?= 0

BIN_DIR := bin
BIN_GOTSL    := $(BIN_DIR)/gotsl
BIN_REVERSE  := $(BIN_DIR)/reverse

# Version metadata
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    := $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')
LDFLAGS := -s -w \
	-X golang-https-rev/pkg/version.Version=$(VERSION) \
	-X golang-https-rev/pkg/version.Commit=$(COMMIT) \
	-X golang-https-rev/pkg/version.Date=$(DATE)

.PHONY: all help build test fmt vet clean run-gotsl run-reverse cover mod

all: build

help:
	@echo "Available targets:"
	@echo "  build          Build gotsl and reverse binaries"
	@echo "  test           Run all tests verbosely"
	@echo "  fmt            Format code (go fmt ./...)"
	@echo "  vet            Run go vet"
	@echo "  clean          Remove built binaries and coverage files"
	@echo "  run-gotsl      Run gotsl with PORT and IFACE (defaults: $(PORT) $(IFACE))"
	@echo "  run-reverse    Run reverse with TARGET and RETRIES (defaults: $(TARGET) $(RETRIES))"
	@echo "  cover          Run tests with coverage and generate coverage.html"
	@echo "  mod            Run 'go mod tidy'"

$(BIN_DIR):
	mkdir -p $(BIN_DIR)

build: $(BIN_DIR)
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BIN_GOTSL) ./cmd/gotsl
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BIN_REVERSE) ./cmd/reverse

test:
	$(GO) test ./... -v

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

clean:
	rm -rf $(BIN_DIR) coverage.out coverage.html

run-gotsl: build
	$(BIN_GOTSL) $(PORT) $(IFACE)

run-reverse: build
	$(BIN_REVERSE) $(TARGET) $(RETRIES)

cover:
	$(GO) test ./... -coverprofile=coverage.out
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage written to coverage.html"

mod:
	$(GO) mod tidy
