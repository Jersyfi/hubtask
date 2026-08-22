# apps/webapp — the to-do application

The product UI, in the browser. Not the website: `apps/website` is hubtask.eu, an information site
with no task management in it. The names matter because `apps/web` would not say which.

This bundle is embedded into the Go binary and served at `/` by `presentation/webui`
([ADR-0028](../../docs/adr/ADR-0028-embedded-web-ui.md)), so the API and the interface always come
from the same commit and cannot be a version apart.

## What must not happen here

* **No framework decision.** It has not been taken, it needs its own ADR, and the scaffold is
  deliberately framework-free so it stays reversible (arc42 §2.2 C-14). Two constraints are
  already fixed for whoever takes it: the content security policy permits neither
  `'unsafe-inline'` nor `'unsafe-eval'`, and every value comes from the design system.
* **No colour, spacing, radius or duration written here.** They come from
  `@hubtask/design-system`; a value that does not exist is added to `tokens.json` or is not
  needed. `pnpm lint` fails on one ([ADR-0029](../../docs/adr/ADR-0029-design-system-tokens.md)).
* **No hand-written API type.** They come from `@hubtask/api-client`, generated from
  `api/openapi.yaml`. If the type you need is not there, change the specification (ADR-0004).
* **No request to a foreign origin.** `connect-src 'self'`, and the fonts ship with the bundle. A
  self-hosted Hubtask contacts nobody on load (ADR-0018).
* **No dependency on `apps/website`.** The two clients share packages, never each other.

## Two things the API decides for you

* **Cursor pagination, never page numbers** — the API has none, so no component may imply them.
* **`/meta/capabilities` is what the client configures itself from**, including which fields may be
  filtered. A hard-coded list will be wrong on somebody's installation.

## How to check a change

```bash
pnpm --filter @hubtask/webapp build
pnpm --filter @hubtask/webapp lint       # no literal values
pnpm --filter @hubtask/webapp typecheck
pnpm --filter @hubtask/webapp test
```

Nothing here is importable from Go, and no `.go` file belongs in this directory.
