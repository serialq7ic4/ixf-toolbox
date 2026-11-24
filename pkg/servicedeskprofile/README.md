# ServiceDesk Profile Contract

`servicedeskprofile` is the shared Go contract for producing and consuming
ixf ServiceDesk profiles. It is intentionally narrower than the ServiceDesk
runtime in `internal/servicedesk`.

## Boundary

The package provides:

- strict profile and Registry decoding, validation, canonical encoding, and
  content hashes;
- ServiceDesk edit URL parsing and read-only `service`/`lading` metadata
  loading;
- profile discovery, live-template resolution, drift classification, and
  deterministic profile rebase;
- verified, read-only Registry fetching.

Pure domain functions do not read environment variables, credential files,
cache directories, or Agent configuration. `MetadataClient` receives an API
key explicitly and only reads form metadata. `RegistryFetcher` receives its
base URL and HTTP client explicitly and only reads Registry files. Their
default HTTP clients ignore proxy environment variables and reject redirects.

The package does not expose ticket instance reads, ticket creation,
processing, or withdrawal. Those operations remain in the private
`internal/servicedesk` runtime.

## Main APIs

- `ParseEditURL`, `LoadTemplate`, and `MetadataClient` read and normalize a
  live form.
- `DiscoverOverlay`, `DecodeOverlay`, `ValidateOverlay`, and `EncodeProfile`
  manage profile documents.
- `CompareBaseline` and `RebaseOverlay` identify drift and migrate approved
  business semantics without silently dropping unresolved customization.
  Before publishing a rebased draft, also call `InspectTemplate` with its
  profile context: it checks the final CLI input contract, including condition
  reference conflicts that are not overlay matcher errors.
- `DecodeRegistry`, `EncodeRegistry`, `ContentAddressedProfilePath`, and
  `RegistryFetcher` implement the shared Registry contract.
- `InspectTemplate` and `ResolveProfileInputContract` produce the same input
  contract consumed by the `ixf` CLI.

All callers must treat profile and Registry bytes as canonical artifacts.
Published profiles are immutable and content-addressed; publication replaces
`index.json` only after every referenced profile is durable.

## External Modules

External tools should pin the `ixf-toolbox` commit that introduced the public
contract. During local development, use an uncommitted `go.work` file. After
the commit is merged, replace the workspace with a module dependency:

```bash
go get github.com/serialq7ic4/ixf-toolbox@<commit>
go mod tidy
```

This records a Go pseudo-version when the commit is not itself a release tag.
Do not commit a local `replace` directive.
