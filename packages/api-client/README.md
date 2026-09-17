# @hubtask/api-client

TypeScript types for the Hubtask API. **Everything in `dist/` is generated. Never edit it.**

The source is [`api/openapi.yaml`](../../api/openapi.yaml), the same file the Go server types come
from ([ADR-0004](../../docs/adr/ADR-0004-api-first-openapi.md)). The specification is changed
first; the code is the result.

```bash
make api-client     # or: pnpm --filter @hubtask/api-client build
```

`dist/` is ignored by git — it is reproducible from the specification and the lockfile, and CI
regenerates it before anything typechecks against it.

## Why this package holds nothing but generated output

[ADR-0027](../../docs/adr/ADR-0027-monorepo-structure.md) deferred a decision to before `1.0.0`:
whether the generated SDKs move into a separately licensed repository, because a client library
under BSL 1.1 is a client library nobody may use in commercial production and therefore nobody
builds on. Keeping this package free of hand-written code means that extraction stays a move
rather than a rewrite.

## Licence: two in one package

The licence half of that decision is made
([ADR-0059](../../docs/adr/ADR-0059-licensing-phases-and-licensing-start.md) §6, deciding
[ADR-0057](../../docs/adr/ADR-0057-sdk-licence-and-extraction.md)); the extraction half stays
open. So this package holds files under two licences, and each file says which in its header:

| | Licence |
|---|---|
| `src/client.gen.ts` — the TypeScript SDK — and `examples/` | **Apache-2.0**, the header written by `tools/sdkgen` |
| `src/index.ts`, the generated types in `dist/`, `scripts/`, this manifest | **BUSL-1.1**, first-party like the rest of the repository |
| `dist/openapi.json`, `dist/events.json` | Apache-2.0 at their source, `api/` |

The Apache-2.0 client imports the first-party types; a third party regenerates those from the
Apache-2.0 contract with `openapi-typescript` rather than taking them from here (ADR-0059
Appendix B). The `license` field of `package.json` names the workspace member — a private,
first-party package — and not the SDK; what a registry would receive is cut from the Apache-2.0
files alone, when and if publication is decided.

`pnpm lint` enforces it: an interface, a hand-built type alias or a runtime export under `src/`
fails the check. If you need something the types do not describe, describe it in
`api/openapi.yaml` and regenerate.

## There is no runtime client yet

Only types. The fetch layer belongs to the sync engine's `Transport` port
([ADR-0033](../../docs/adr/ADR-0033-shared-client-architecture.md)) and arrives with that work
package — not here as a side effect, and never hand-written.
