.PHONY: build build-mac build-linux build-windows test clean

VERSION := 0.2.0
BINARY  := clipsync

build: build-mac build-linux build-windows

build-mac:
	@mkdir -p dist
	GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w -X main.version=$(VERSION)" -o dist/$(BINARY)-mac-arm64 .
	GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w -X main.version=$(VERSION)" -o dist/$(BINARY)-mac-amd64 .

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
