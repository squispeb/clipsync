# Releasing

Releases are built by [GoReleaser](https://goreleaser.com) in `.github/workflows/release.yml` when a `v*` tag is pushed:

```bash
git tag -a v0.5.1 -m "v0.5.1" && git push origin v0.5.1
```

The workflow runs on a macOS runner (the macOS binaries need cgo/AppKit; Linux and Windows are pure Go) and:

1. runs the tests;
2. builds linux, darwin and windows for amd64 and arm64, with the version taken from the tag;
3. publishes the GitHub release with archives and `checksums.txt`;
4. pushes the Homebrew cask to [squispeb/homebrew-tap](https://github.com/squispeb/homebrew-tap) (`Casks/clipsync.rb`), using the `HOMEBREW_TAP_DEPLOY_KEY` secret, a write deploy key on that repo;
5. creates build provenance attestations for every archive;
6. attaches `winget-manifests.zip`.

mise needs nothing extra: `mise use -g github:squispeb/clipsync` reads the release directly.

## winget

GoReleaser writes the manifests but doesn't submit them (`skip_upload: true`), because opening a PR to `microsoft/winget-pkgs` needs a personal access token. Submit each release by hand:

```bash
gh release download v0.5.1 -p winget-manifests.zip && unzip winget-manifests.zip
gh repo fork microsoft/winget-pkgs --clone=false   # once
# copy winget/manifests/s/Squispeb/ClipSync/<version>/ into a branch of your fork
# under manifests/s/Squispeb/ClipSync/<version>/ and open a PR to microsoft/winget-pkgs
```

To automate it, create a fine-grained token with access to your winget-pkgs fork, store it as a repo secret, and set `winget.skip_upload: false` with `repository.pull_request.enabled: true` in `.goreleaser.yaml`.

## Checklist

- `make test` passes, and CI is green on master.
- Bump nothing by hand: the version comes from the tag (`main.version` is only the fallback for local builds; keep it at the upcoming version).
