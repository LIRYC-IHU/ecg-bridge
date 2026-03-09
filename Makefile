TOOLS     := $(notdir $(wildcard cmd/*))
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS   := -ldflags "-X main.version=$(VERSION) -s -w"
BIN_DIR   := bin

.PHONY: all build test lint clean install $(TOOLS)

all: build

## Build all tools for the current platform
build: $(TOOLS)

$(TOOLS):
	@mkdir -p $(BIN_DIR)
	go build $(LDFLAGS) -o $(BIN_DIR)/$@ ./cmd/$@

## Run tests
test:
	go test ./...

## Run go vet + staticcheck (if available)
lint:
	go vet ./...
	@command -v staticcheck >/dev/null 2>&1 && staticcheck ./... || true

## Install all tools to $GOPATH/bin
install:
	@for t in $(TOOLS); do \
		echo "Installing $$t..."; \
		go install $(LDFLAGS) ./cmd/$$t; \
	done

## Remove build artifacts
clean:
	rm -rf $(BIN_DIR)

help:
	@echo "Targets:"
	@echo "  build    Build all tools ($(TOOLS))"
	@echo "  test     Run unit tests"
	@echo "  lint     Run go vet (+ staticcheck if installed)"
	@echo "  install  Install all tools to GOPATH/bin"
	@echo "  clean    Remove bin/"
