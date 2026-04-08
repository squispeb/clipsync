.PHONY: build build-mac build-windows test clean

VERSION := 0.1.0
BINARY  := cliplink

build: build-mac build-windows

build-mac:
	@mkdir -p dist
	GOOS=darwin GOARCH=arm64 go build -o dist/$(BINARY)-mac-arm64 .
	GOOS=darwin GOARCH=amd64 go build -o dist/$(BINARY)-mac-amd64 .

build-windows:
	@mkdir -p dist
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o dist/$(BINARY)-windows-amd64.exe .

test:
	go test -v ./...

clean:
	rm -rf dist/
