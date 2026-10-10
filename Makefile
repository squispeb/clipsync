.PHONY: build test snapshot clean

# Local build for this machine. Releases are built by GoReleaser
# (.goreleaser.yaml) in .github/workflows/release.yml.
# macOS needs cgo (AppKit); Linux and Windows are pure Go.
build:
	@mkdir -p dist
	go build -ldflags="-s -w" -o dist/clipsync .

test:
	go test -race ./...

# Full release dry run (macOS only, since the macOS binaries need cgo).
snapshot:
	goreleaser release --snapshot --clean --skip=publish

clean:
	rm -rf dist/
