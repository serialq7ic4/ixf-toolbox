# Release

`ixf-toolbox` uses tagged GitHub Releases with Go binary artifacts. Python wheel
and source distribution artifacts stopped being published in v2.6.0, the Python
runtime/package implementation was removed in v3.0.0, and the Python test
harness was removed in v3.1.0.

## Changelog

Every release must have a human-written, non-empty section in `CHANGELOG.md`.

Before tagging:

1. Update `VERSION` and the Go CLI version.
2. Add a dated `CHANGELOG.md` section.
3. Keep entries focused on supported behavior, safety changes, and migration notes.
4. Verify the release notes section is non-empty:

```bash
version=X.Y.Z
awk -v version="${version}" '$0 ~ "^## " version "([[:space:]-]|$)" { found=1; next } found && /^## / { exit } found { print }' CHANGELOG.md | sed '/^[[:space:]]*$/d'
```

## Local Checks

```bash
RELEASE_VERSION=X.Y.Z
go test ./...
go vet ./...
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${RELEASE_VERSION}" -o /tmp/ixf-go ./cmd/ixf
scripts/smoke-go-binary.sh /tmp/ixf-go "${RELEASE_VERSION}"
rm -rf dist build
```

## Tag

```bash
git tag vX.Y.Z
git push origin main
git push origin vX.Y.Z
```

The GitHub Actions release workflow validates the tag against `VERSION`, runs Go tests and `go vet`, builds Go artifacts, extracts release notes from `CHANGELOG.md`, and creates the GitHub Release.

After release, confirm:

- The release body matches the changelog section.
- The Go binaries and checksum file are attached for macOS, Linux, and Windows.
- A clean current-platform Go binary download can run `ixf --version`, `ixf --help`, `ixf setup skills --runtimes codex --json`, and a local `ixf docs read`.

Do not publish Python package artifacts; supported release assets are Go
binaries and checksums only.
