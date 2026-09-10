# Release

`ixf-toolbox` uses tagged GitHub Releases with Go binary artifacts. Python wheel
and source distribution artifacts stopped being published in v2.6.0, the Python
runtime/package implementation was removed in v3.0.0, and the Python test
harness was removed in v3.1.0.

## Changelog

Every release must have a human-written, non-empty section in `CHANGELOG.md`.

Before tagging:

1. Update `VERSION`; the Go CLI and generated plugin manifests embed this file.
2. Add a dated `CHANGELOG.md` section.
3. Keep entries focused on supported behavior, safety changes, and migration notes.
4. Regenerate plugin packages and verify the release notes section is non-empty:

```bash
go run ./cmd/pluginpack
go run ./cmd/pluginpack --check
version=X.Y.Z
awk -v version="${version}" '$0 ~ "^## " version "([[:space:]-]|$)" { found=1; next } found && /^## / { exit } found { print }' CHANGELOG.md | sed '/^[[:space:]]*$/d'
```

## Local Checks

```bash
go test ./...
go vet ./...
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /tmp/ixf-go ./cmd/ixf
scripts/smoke-go-binary.sh /tmp/ixf-go "$(cat VERSION)"
scripts/smoke-native-plugins.sh
/tmp/ixf-go deps install --dry-run --json
rm -rf dist build
```

## Tag

```bash
git tag vX.Y.Z
HTTPS_PROXY=http://127.0.0.1:7890 HTTP_PROXY=http://127.0.0.1:7890 git push origin main
HTTPS_PROXY=http://127.0.0.1:7890 HTTP_PROXY=http://127.0.0.1:7890 git push origin vX.Y.Z
```

The GitHub Actions release workflow validates the tag against `VERSION`, runs Go tests and `go vet`, builds Go artifacts, extracts release notes from `CHANGELOG.md`, and creates the GitHub Release.

After release, confirm:

- The release body matches the changelog section.
- The Go binaries and checksum file are attached for macOS, Linux, and Windows.
- Generated Codex and Claude plugin packages match canonical skills and the tagged `VERSION`.
- A clean current-platform Go binary download can run `ixf --version`, `ixf --help`, `ixf deps install --dry-run --json`, and a local `ixf docs read`.
- For docs update changes, review `docs/docs-update.md` and run the relevant mocked CLI tests before tagging; live document updates must use a non-sensitive test document and an explicit dry-run/apply confirmation.

Do not publish Python package artifacts; supported release assets are Go
binaries and checksums only.
