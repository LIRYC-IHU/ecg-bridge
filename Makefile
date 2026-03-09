TOOLS     := $(notdir $(wildcard cmd/*))
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS   := -ldflags "-X main.version=$(VERSION) -s -w"
BIN_DIR   := bin
SRCS      := $(shell find . -name "*.go" -not -path "./vendor/*")

.PHONY: all clean fclean re test lint install help

all: $(addprefix $(BIN_DIR)/, $(TOOLS))

# No relink: only rebuild if sources are newer than the binary
$(BIN_DIR)/%: $(SRCS)
	@mkdir -p $(BIN_DIR)
	go build $(LDFLAGS) -o $@ ./cmd/$*

test:
	go test ./...

lint:
	go vet ./...
	@command -v staticcheck >/dev/null 2>&1 && staticcheck ./... || true

install:
	@for t in $(TOOLS); do \
		echo "Installing $$t..."; \
		go install $(LDFLAGS) ./cmd/$$t; \
	done

clean:
	rm -rf $(BIN_DIR)

fclean: clean

re: fclean all

help:
	@echo "Targets:"
	@echo "  all      Build all tools ($(TOOLS))"
	@echo "  clean    Remove bin/"
	@echo "  fclean   Same as clean"
	@echo "  re       fclean + all"
	@echo "  test     Run unit tests"
	@echo "  lint     Run go vet (+ staticcheck if installed)"
	@echo "  install  Install all tools to GOPATH/bin"
