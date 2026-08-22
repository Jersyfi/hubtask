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

[ADR-0027](../../docs/adr/ADR-0027-monorepo-structure.md) defers a decision to before `1.0.0`:
whether the generated SDKs move into a separately licensed repository, because a client library
under BSL 1.1 is a client library nobody may use in commercial production and therefore nobody
builds on. Keeping this package free of hand-written code means that extraction stays a move
rather than a rewrite.

`pnpm lint` enforces it: an interface, a hand-built type alias or a runtime export under `src/`
fails the check. If you need something the types do not describe, describe it in
`api/openapi.yaml` and regenerate.

## There is no runtime client yet

Only types. Which fetch layer the webapp uses is part of the frontend framework decision that has
not been taken, and picking one here would prejudge it.
