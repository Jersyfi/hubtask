# apps/website — the project website

hubtask.eu: what Hubtask is, how it is licensed, and the way into the documentation. **Information
only.** No task management, no API client, no account.

It is not embedded into the binary and never will be — it has no contract with the server, so it
has no reason to be inside it ([ADR-0028](../../docs/adr/ADR-0028-embedded-web-ui.md)). `make
website` builds it into `dist/`, and `.github/workflows/website.yml` publishes that directory to
the hubtask.eu webspace (IONOS) over SFTP on every push to `main` that touches it. The workflow
skips politely on a fork or an installation where the deploy variables are not configured.

## What must not happen here

* **No API calls.** If this page needs data from an installation, the requirement is wrong: this
  is a brochure, and `apps/webapp` is the application.
* **From `@hubtask/api-client`, the document and nothing else.** The site takes
  `openapi.json` and `events.json` from it at build time to prerender `/developers/api/` (P-01,
  `src/lib/api/`), which is the contract as documentation. It takes no type from it and makes no
  request with it; a page that imported `paths` or `components` would be a page about to call
  something. The design system is the other dependency, and there is no third.
* **No colour, spacing, radius or duration written here** — same rule as everywhere, same lint
  ([ADR-0029](../../docs/adr/ADR-0029-design-system-tokens.md)).
* **The framework is decided**: Svelte 5 with SvelteKit and `adapter-static`, fully prerendered
  ([ADR-0030](../../docs/adr/ADR-0030-svelte-frontend-framework.md)). Build-time Node only —
  the output is plain static files, and nothing here runs a server.
* **No claim about the licence that `LICENSE` does not make.** Hubtask is open source under the
  Apache License 2.0 ([ADR-0080](../../docs/adr/ADR-0080-hubtask-is-apache-2-0.md)), and the site
  says so plainly: "open source", "Apache-2.0", "free for any use". The name is not covered by the
  licence ([`TRADEMARK.md`](../../TRADEMARK.md)). What the site never says: a price, a paid
  edition, a support period, a deadline or a maintenance commitment — the project is provided as
  is.

## How to check a change

```bash
pnpm --filter @hubtask/website build
pnpm --filter @hubtask/website lint
pnpm --filter @hubtask/website typecheck
```

Nothing here is importable from Go, and no `.go` file belongs in this directory.
