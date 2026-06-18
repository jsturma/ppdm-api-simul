BINARY      := ppdm-simulator
CMD         := ./cmd/ppdm-simulator
DIST        := dist
LDFLAGS     := -s -w

GOOS   ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

NATIVE_BIN := $(DIST)/$(BINARY)

.PHONY: all help build run test clean dist release \
	linux-amd64 linux-arm64 darwin-amd64 darwin-arm64

all: build

help:
	@echo "PPDM API Simulator"
	@echo ""
	@echo "Usage:"
	@echo "  make build          Build for current OS/arch -> $(NATIVE_BIN)"
	@echo "  make release        Build all release binaries -> $(DIST)/"
	@echo "  make linux-amd64    Build Linux x86_64 binary"
	@echo "  make linux-arm64    Build Linux ARM64 binary"
	@echo "  make darwin-amd64   Build macOS Intel binary"
	@echo "  make darwin-arm64   Build macOS Apple Silicon binary"
	@echo "  make run            Run without building (go run)"
	@echo "  make test           Run tests"
	@echo "  make clean          Remove $(DIST)/"

dist:
	@mkdir -p $(DIST)

build: dist
	go build -ldflags="$(LDFLAGS)" -o $(NATIVE_BIN) $(CMD)

release: dist linux-amd64 linux-arm64 darwin-amd64 darwin-arm64

linux-amd64: dist
	GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(DIST)/$(BINARY)-linux-amd64 $(CMD)

linux-arm64: dist
	GOOS=linux GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(DIST)/$(BINARY)-linux-arm64 $(CMD)

darwin-amd64: dist
	GOOS=darwin GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(DIST)/$(BINARY)-darwin-amd64 $(CMD)

darwin-arm64: dist
	GOOS=darwin GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(DIST)/$(BINARY)-darwin-arm64 $(CMD)

run:
	go run $(CMD)

test:
	go test ./...

clean:
	rm -rf $(DIST)
