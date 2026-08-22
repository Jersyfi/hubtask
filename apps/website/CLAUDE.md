# apps/website — the project website

hubtask.eu: what Hubtask is, how it is licensed, and the way into the documentation. **Information
only.** No task management, no API client, no account.

It is not embedded into the binary and never will be — it has no contract with the server, so it
has no reason to be inside it ([ADR-0028](../../docs/adr/ADR-0028-embedded-web-ui.md)). `make
website` builds it into a directory; where that directory is deployed is not decided yet.

## What must not happen here

* **No API calls.** If this page needs data from an installation, the requirement is wrong: this
  is a brochure, and `apps/webapp` is the application.
* **No dependency on `@hubtask/api-client`.** Only the design system.
* **No colour, spacing, radius or duration written here** — same rule as everywhere, same lint
  ([ADR-0029](../../docs/adr/ADR-0029-design-system-tokens.md)).
* **No framework decision**, for the same reason as the webapp: it has not been taken.
* **No claim about the licence that `LICENSE` does not make.** BSL 1.1 with a Change Date to
  Apache-2.0 after three years is "source available", not "open source", and
  [ADR-0013](../../docs/adr/ADR-0013-licensing.md) is explicit about that. Saying otherwise on the
  website is a legal problem, not a wording preference.

## How to check a change

```bash
pnpm --filter @hubtask/website build
pnpm --filter @hubtask/website lint
pnpm --filter @hubtask/website typecheck
```

Nothing here is importable from Go, and no `.go` file belongs in this directory.
