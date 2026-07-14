# Release

`ixf-toolbox` uses tagged GitHub Releases with wheel and source distribution artifacts.

## Changelog

Every release must have a human-written, non-empty section in `CHANGELOG.md`.

Before tagging:

1. Update `pyproject.toml` and `src/ixf_toolbox/__init__.py`.
2. Add a dated `CHANGELOG.md` section.
3. Keep entries focused on supported behavior, safety changes, and migration notes.
4. Verify the release notes section can be extracted:

```bash
python scripts/extract_changelog.py X.Y.Z CHANGELOG.md
```

## Local Checks

```bash
python -m compileall -q src
python -m pytest -q
python -m ruff check .
go test ./...
rm -rf dist build
python -m build
scripts/smoke.sh
```

## Tag

```bash
git tag vX.Y.Z
git push origin main
git push origin vX.Y.Z
```

The GitHub Actions release workflow validates the tag against the package version, runs tests and lint, builds artifacts, extracts release notes from `CHANGELOG.md`, and creates the GitHub Release.

After release, confirm:

- The release body matches the changelog section.
- The wheel and source distribution are attached.
- A clean wheel installation can run `ixf --version`, `ixf --help`, and `ixf setup skills --runtimes codex --json`.

Do not publish to PyPI until support status and privacy documentation are current.
