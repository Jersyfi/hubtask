# packages/api-client — generated, and nothing else

Everything under `dist/` comes from `api/openapi.yaml` by way of `make api-client`. The
specification is the source and the code is the result
([ADR-0004](../../docs/adr/ADR-0004-api-first-openapi.md)).

## What must not happen here

* **No hand-written type.** An interface, a hand-built type alias or a runtime export under `src/`
  fails `pnpm lint`. If the type you need is missing, it is missing from the contract: change
  `api/openapi.yaml`, run `make generate` **and** `make api-client`, and put both halves in one
  pull request.
* **No fetch layer, no client class, no retry logic — yet.** The fetch layer belongs to the sync
  engine's `Transport` port ([ADR-0033](../../docs/adr/ADR-0033-shared-client-architecture.md))
  and arrives with that work package, not here as a side effect of other work.
* **No committed output.** `dist/` is ignored: it is reproducible from the specification and the
  lockfile, and a committed copy would be a second description of the contract to keep in step.
* **No dependency on anything under `apps/`.** A package that knows about an application is not a
  shared package.

## Why the constraint is this strict

[ADR-0027](../../docs/adr/ADR-0027-monorepo-structure.md) defers a decision to before `1.0.0`:
whether the generated SDKs move to a separately licensed repository, because a client library
under BSL 1.1 is one nobody may use in commercial production and therefore nobody builds on.
Keeping this package free of hand-written code is what keeps that extraction a move rather than a
rewrite.

## How to check a change

```bash
make api-client
pnpm --filter @hubtask/api-client lint
pnpm --filter @hubtask/api-client typecheck
pnpm -r typecheck        # the consumers still agree with the regenerated contract
```

Nothing here is importable from Go, and no `.go` file belongs in this directory.
