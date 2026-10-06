# apps/website — the project website

hubtask.eu: what Hubtask is, how it is licensed, and the way into the documentation. Information
only — no task management, no account. It is never embedded into the binary; `make website` builds
it into `dist/`, and `.github/workflows/website.yml` publishes that on a push to `main`.

## What must not happen here

* **No API call.** If a page needs data from an installation, the requirement is wrong:
  `apps/webapp` is the application.
* **From `@hubtask/api-client`, the document and nothing else.** The site reads `openapi.json` and
  `events.json` at build time to prerender `/developers/api/` (`src/lib/api/`). It imports no type
  from the package and makes no request with it; a page that imported `paths` or `components`
  would be a page about to call something. The design system is the only other workspace
  dependency.
* **No colour, spacing, radius or duration written here** (rule 15).
* **No other framework or runtime.** Svelte 5 with SvelteKit and `adapter-static`, fully
  prerendered; Node runs at build time only, and nothing here runs a server.
* **No claim about the licence that `LICENSE` does not make.** Hubtask is open source under the
  Apache License 2.0, and the site says so plainly: "open source", "Apache-2.0", "free for any
  use". The name is not covered by the licence (`TRADEMARK.md`). The site never states a price, a
  paid edition, a support period, a deadline or a maintenance commitment — the project is provided
  as is.
* **No `.go` file** (rule 14).

## How to check a change

```bash
make website                              # builds it and what it depends on, as the workflow does
pnpm --filter @hubtask/website lint
pnpm --filter @hubtask/website typecheck
pnpm --filter @hubtask/website test
```
