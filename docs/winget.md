# Winget Packaging

ClipSync can be published to `winget` as a portable package.

## Required release assets

- `clipsync-windows-portable.zip`
- `clipsync-windows-amd64.exe` inside the zip

## Recommended manifest shape

- `InstallerType: zip`
- `NestedInstallerType: portable`
- `PortableCommandAlias: clipsync`
- `ArchiveBinariesDependOnPath: true`

## Release flow

1. Build the Windows portable zip.
2. Publish the release asset to GitHub Releases.
3. Generate a manifest with `wingetcreate new`.
4. Submit the manifest to `microsoft/winget-pkgs`.

## Local test

Run the helper script from the repo root on Windows:

```powershell
.\scripts\test-winget-local.ps1
```

It starts a tiny localhost file server, enables local manifests, and runs `winget install` against the local manifest folder.

## Notes

- Winget is the installer entrypoint, not `clipsync install`.
- The package should install a command alias so users can run `clipsync` after install.
- The repo should be public for normal winget publishing.
