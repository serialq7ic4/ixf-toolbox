## Summary

Describe the problem and user-visible change.

## Scope

- [ ] Document reading or conversion
- [ ] Document publishing
- [ ] OKR reading
- [ ] OKR writing
- [ ] Cookie export or diagnostics
- [ ] Skill install or update flow
- [ ] Packaging, CI, or release
- [ ] Documentation only

## Safety

- [ ] Remote mutations remain dry-run by default.
- [ ] Destructive behavior remains explicit.
- [ ] No cookies, CSRF tokens, private URLs, people, internal identifiers, raw responses, or private content are included.
- [ ] Errors and diagnostics remain redacted.
- [ ] Generated private artifacts are not committed.

## Verification

```bash
python -m compileall -q src
python -m pytest -q
python -m ruff check .
```

If packaging or release behavior changed:

```bash
python -m build
scripts/smoke.sh
```

## Documentation

- [ ] README / docs updated, or not needed because:
- [ ] CHANGELOG updated for user-visible changes, or not needed because:
