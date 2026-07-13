# ixf-toolbox Design

## Purpose

`ixf-toolbox` is the unified local package for i讯飞 daily workflow automation. Users install one package and get one CLI, `ixf`, plus domain-specific Codex and Claude Code skills for authorized cloud document and OKR work.

The first release is a compatibility bridge. It preserves the working `ixunfei-docx-reader` and `ixunfei-docx-writer` implementations as runtime dependencies while introducing the stable Toolbox surface. Later releases migrate shared cookie/session, document, and OKR code into `ixf_toolbox.core` without changing the user-facing `ixf` commands.

## Naming

- Product: `i讯飞 Toolbox`
- Repository: `ixf-toolbox`
- Python distribution: `ixf-toolbox`
- Python package: `ixf_toolbox`
- CLI: `ixf`
- Skills: `ixf-docs-reader`, `ixf-docs-writer`, `ixf-okr-reader`, `ixf-okr-writer`

## Architecture

The package has three layers:

- `ixf_toolbox.cli`: the stable command surface for agents and humans.
- `ixf_toolbox.delegate`: compatibility bridge that forwards commands to installed legacy engines.
- `ixf_toolbox.setup`: skill wrapper installer for Codex and Claude Code.

The initial command mapping is:

- `ixf docs read ...` delegates to `ixfdoc read ...`.
- `ixf docs outline ...` delegates to `ixfdoc outline ...`.
- `ixf docs chunk ...` delegates to `ixfdoc chunk ...`.
- `ixf docs cleanup ...` delegates to `ixfdoc cleanup ...`.
- `ixf docs inspect ...` delegates to `ixfdoc inspect ...`.
- `ixf docs publish ...` delegates to `ixfwrite docx publish ...`.
- `ixf okr read ...` delegates to `ixfdoc read ...`.
- `ixf okr write ...` delegates to `ixfwrite okr write ...`.
- `ixf cookies export ...` delegates to `ixfwrite cookies export ...`.
- `ixf doctor ...` delegates to `ixfwrite doctor ...`.
- `ixf setup skills ...` installs Toolbox skill wrappers.
- `ixf update check ...` checks the latest Toolbox release.
- `ixf update skills ...` refreshes installed Toolbox skill wrappers from the current package.

## Permissions Model

Skills are split by resource domain and permission:

- `docs-reader`: reads and exports authorized cloud document content.
- `docs-writer`: creates or modifies authorized cloud documents.
- `okr-reader`: reads and analyzes authorized OKR pages.
- `okr-writer`: writes confirmed OKR content.

Reader skills must not perform writes. Writer skills must require explicit user confirmation and should prefer dry-run command examples before `--apply`.

## Compatibility

The old repositories stay available during migration:

- `serialq7ic4/ixunfei-docx-reader`
- `serialq7ic4/ixunfei-docx-writer`

`ixf-toolbox` depends on their release wheels for the initial bridge. Old commands remain usable:

- `ixfdoc`
- `ixfwrite`

New user-facing docs should prefer `ixf`. Old repositories can later add deprecation notices once Toolbox reaches feature parity.

## Security

The public repository must not include real tenant URLs, document IDs, OKR IDs, person names, cookies, CSRF tokens, passwords, or private response payloads. Examples use placeholder domains such as `https://tenant.example.test`.

CLI errors must avoid printing cookie values or private API payloads. Skills must tell agents to avoid committing generated private artifacts unless the user explicitly asks.

## Release Policy

Initial release is `v0.1.0`. It can be published once these checks pass:

- `python -m pytest`
- `python -m build`
- install generated wheel in a clean environment
- `ixf --version`
- `ixf setup skills --runtimes codex --json`

Changelog entries are required for every release. GitHub releases should include the wheel, sdist, and concise migration notes.
