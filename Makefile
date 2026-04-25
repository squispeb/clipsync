.PHONY: build build-mac build-mac-native build-linux build-windows test clean

VERSION := 0.3.0
BINARY  := clipsync

# NOTE: macOS binaries MUST be built natively on macOS because
# golang.design/x/clipboard requires CGO on Darwin.
# Cross-compiling from Linux produces a broken binary.
build: build-mac-native build-linux build-windows

# Cross-compiled macOS binaries (BROKEN — no CGO support)
# Only use for testing non-clipboard code.
build-mac-cross:
	@mkdir -p dist
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=$(VERSION)" -o dist/$(BINARY)-mac-arm64 .
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=$(VERSION)" -o dist/$(BINARY)-mac-amd64 .

# Build macOS binaries natively (requires macOS + Xcode CLI tools)
build-mac-native:
	@mkdir -p dist
	go build -ldflags="-s -w -X main.version=$(VERSION)" -o dist/$(BINARY)-mac-$(shell uname -m) .

build-linux:
	@mkdir -p dist
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w -X main.version=$(VERSION)" -o dist/$(BINARY)-linux-amd64 .
	GOOS=linux GOARCH=arm64 go build -ldflags="-s -w -X main.version=$(VERSION)" -o dist/$(BINARY)-linux-arm64 .

build-windows:
	@mkdir -p dist
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=$(VERSION)" -o dist/$(BINARY)-windows-amd64.exe .

test:
	go test -v ./...

clean:
	rm -rf dist/
