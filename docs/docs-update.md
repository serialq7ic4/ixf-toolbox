# Docs Update

`ixf docs update` updates an existing authorized docx document from local
Markdown. It uses `replace_body` mode: the original document URL, permissions,
and location stay unchanged, while the body blocks are replaced.

## Safety Boundary

Always dry-run first:

```bash
ixf docs update notes/review.md \
  --url https://tenant.example.test/docx/example \
  --cookies /tmp/ixf_cookies.json \
  --dry-run
```

The dry-run reads the target document state and reports:

- `operation:update_docx`
- `mode:replace_body`
- `destructive:true`
- current and planned top-level block counts
- complex blocks detected in the existing body
- `tableCount`, `tableBlockType`, and `tableFallbackCount`; Markdown tables are
  written as native docx table blocks when possible

Apply only after reviewing the plan:

```bash
ixf docs update notes/review.md \
  --url https://tenant.example.test/docx/example \
  --cookies /tmp/ixf_cookies.json \
  --require "critical content" \
  --apply
```

After apply, inspect the `verify` object in the JSON response. Treat
`verify.ok=false`, non-empty `verify.missingRequiredText`, or
`verify.emptyCalloutCount>0` as a failed write that needs investigation before
claiming success. Do not rely on a successful HTTP write alone.

## Complex Blocks

Complex blocks include content outside the Markdown writer's supported block
set, such as images, embedded sheets, or other rich app blocks. By default,
`ixf docs update --apply` refuses to replace complex blocks.

If the user has explicitly confirmed that losing those complex blocks is
acceptable, pass the destructive override:

```bash
ixf docs update notes/review.md \
  --url https://tenant.example.test/docx/example \
  --cookies /tmp/ixf_cookies.json \
  --allow-complex-replace \
  --apply
```

## What It Does Not Do

`ixf docs update` does not change document permissions, does not move the document,
does not rename the document, and does not edit comments. It is not a semantic
diff editor; it replaces the existing body with Markdown-generated blocks.
