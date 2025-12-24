# Default goal
.DEFAULT_GOAL := help

GO ?= go
PORT ?= 9001
IFACE ?= 0.0.0.0
TARGET ?= 127.0.0.1:9001
RETRIES ?= 0

BIN_DIR := bin
BIN_GOTSL    := $(BIN_DIR)/gotsl
BIN_GOTSR    := $(BIN_DIR)/gotsr

# Version metadata
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    := $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')
LDFLAGS := -s -w \
	-X github.com/frjcomp/gots/pkg/version.Version=$(VERSION) \
	-X github.com/frjcomp/gots/pkg/version.Commit=$(COMMIT) \
	-X github.com/frjcomp/gots/pkg/version.Date=$(DATE)

.PHONY: all help build test fmt vet clean run-gotsl run-gotsr cover mod

all: build

help:
	@echo "Available targets:"
	@echo "  build          Build gotsl and gotsr binaries"
	@echo "  test           Run all tests verbosely"
	@echo "  fmt            Format code (go fmt ./...)"
	@echo "  vet            Run go vet"
	@echo "  clean          Remove built binaries and coverage files"
	@echo "  run-gotsl      Run gotsl with --port and --interface (defaults: $(PORT) $(IFACE))"
	@echo "  run-gotsr      Run gotsr with --target and --retries (defaults: $(TARGET) $(RETRIES))"
	@echo "  cover          Run tests with coverage and generate coverage.html"
	@echo "  mod            Run 'go mod tidy'"

$(BIN_DIR):
	mkdir -p $(BIN_DIR)

build: $(BIN_DIR)
	CGO_ENABLED=0 $(GO) build -ldflags "$(LDFLAGS)" -o $(BIN_GOTSL) ./cmd/gotsl
	CGO_ENABLED=0 $(GO) build -ldflags "$(LDFLAGS)" -o $(BIN_GOTSR) ./cmd/gotsr

test:
	$(GO) test ./... -v

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

clean:
	rm -rf $(BIN_DIR) coverage.out coverage.html

l: build
	$(BIN_GOTSL) --port $(PORT) --interface $(IFACE)

r: build
	$(BIN_GOTSR) --target $(TARGET) --retries $(RETRIES)

cover:
	$(GO) test ./... -coverprofile=coverage.out
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage written to coverage.html"

mod:
	$(GO) mod tidy
