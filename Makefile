BINARY_NAME=sticker_go
BUILD_DIR=dist

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
TIME ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS=-ldflags "-X github.com/libost/sticker_go/version.Version=$(VERSION) -X github.com/libost/sticker_go/version.GitCommit=$(COMMIT) -X github.com/libost/sticker_go/version.BuildTime=$(TIME)"

.PHONY: all help tidy vet test build build-linux-x64 build-linux-x64-v3 build-linux-arm64 build-darwin-x64 build-darwin-x64-v3 build-darwin-arm64 build-windows-x64 build-windows-x64-v3 build-windows-arm64 clean

all: tidy vet test build
	
help:
	@echo "Available targets:"
	@echo "  all       - Run tidy, vet, test, and build"
	@echo "  tidy      - Run go mod tidy"
	@echo "  vet       - Run go vet"
	@echo "  test      - Run go test"
	@echo "  build     - Build the application"
	@echo "  build-linux-x64   - Build for Linux (x64)"
	@echo "  build-linux-x64-v3   - Build for Linux (x64) with v3 optimizations"
	@echo "  build-linux-arm64 - Build for Linux (arm64)"
	@echo "  build-darwin-x64  - Build for macOS (x64)"
	@echo "  build-darwin-x64-v3  - Build for macOS (x64) with v3 optimizations"
	@echo "  build-darwin-arm64 - Build for macOS (arm64)"
	@echo "  build-windows-x64 - Build for Windows (x64)"
	@echo "  build-windows-x64-v3 - Build for Windows (x64) with v3 optimizations"
	@echo "  build-windows-arm64 - Build for Windows (arm64)"
	@echo "  clean     - Remove the build directory"

tidy:
	go mod tidy

vet:
	go vet ./...

test:
	go test -v ./...

build:
	mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) .

build-all: build-linux-x64 build-linux-x64-v3 build-linux-arm64 build-darwin-x64 build-darwin-x64-v3 build-darwin-arm64 build-windows-x64 build-windows-x64-v3 build-windows-arm64
	@echo "All builds completed successfully."

build-linux-x64:
	mkdir -p $(BUILD_DIR)/linux-amd64
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/linux-amd64/$(BINARY_NAME)-linux-amd64 .

build-linux-x64-v3:
	mkdir -p $(BUILD_DIR)/linux-amd64-v3
	GOOS=linux GOARCH=amd64 GOAMD64=v3 go build $(LDFLAGS) -o $(BUILD_DIR)/linux-amd64-v3/$(BINARY_NAME)-linux-amd64-v3 .

build-linux-arm64:
	mkdir -p $(BUILD_DIR)/linux-arm64
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/linux-arm64/$(BINARY_NAME)-linux-arm64 .

build-darwin-x64:
	mkdir -p $(BUILD_DIR)/darwin-amd64
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/darwin-amd64/$(BINARY_NAME)-darwin-amd64 .

build-darwin-x64-v3:
	mkdir -p $(BUILD_DIR)/darwin-amd64-v3
	GOOS=darwin GOARCH=amd64 GOAMD64=v3 go build $(LDFLAGS) -o $(BUILD_DIR)/darwin-amd64-v3/$(BINARY_NAME)-darwin-amd64-v3 .

build-darwin-arm64:
	mkdir -p $(BUILD_DIR)/darwin-arm64
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/darwin-arm64/$(BINARY_NAME)-darwin-arm64 .

build-windows-x64:
	mkdir -p $(BUILD_DIR)/windows-amd64
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/windows-amd64/$(BINARY_NAME)-windows-amd64.exe .

build-windows-x64-v3:
	mkdir -p $(BUILD_DIR)/windows-amd64-v3
	GOOS=windows GOARCH=amd64 GOAMD64=v3 go build $(LDFLAGS) -o $(BUILD_DIR)/windows-amd64-v3/$(BINARY_NAME)-windows-amd64-v3.exe .

build-windows-arm64:
	mkdir -p $(BUILD_DIR)/windows-arm64
	GOOS=windows GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/windows-arm64/$(BINARY_NAME)-windows-arm64.exe .

clean:
	rm -rf $(BUILD_DIR)