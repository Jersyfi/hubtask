# apps/website — the project website

hubtask.eu: what Hubtask is, how it is licensed, and the way into the documentation. Information
only — no task management, no account. It is never embedded into the binary; `make website` builds
it into `dist/`, and `.github/workflows/website.yml` publishes that on a push to `main`.

## What must not happen here

* **No API call.** If a page needs data from an installation, the requirement is wrong:
  `apps/webapp` is the application. `[partial: ci:node; open: a request made at build time]`
* **From `@hubtask/api-client`, the document and nothing else.** The site reads `openapi.json` and
  `events.json` at build time to prerender `/developers/api/` (`src/lib/api/`). It imports no type
  from the package and makes no request with it; a page that imported `paths` or `components`
  would be a page about to call something. The design system is the only other workspace
  dependency. `[owner]`
* **No colour, spacing, radius or duration written here** (rule 15).
  `[partial: ci:node; open: named colours, the ignore marker]`
* **No other framework or runtime.** Svelte 5 with SvelteKit and `adapter-static`, fully
  prerendered; Node runs at build time only, and nothing here runs a server.
  `[partial: ci:node; open: the choice of a dependency]`
* **No claim about the licence that `LICENSE` does not make.** Hubtask is open source under the
  Apache License 2.0, and the site says so plainly: "open source", "Apache-2.0", "free for any
  use". The name is not covered by the licence (`TRADEMARK.md`). The site never states a price, a
  paid edition, a support period, a deadline or a maintenance commitment — the project is provided
  as is. `[partial: gate-architecture; open: wording about price, support or deadlines]`
* **No capability left unnamed, and none claimed early.** The site names everything Hubtask does or
  has decided to do — otherwise what the project can do gets lost. A built capability is stated
  plainly; a decided one that is not built carries a *Planned* chip and is never described as
  working. `/use-cases/` covers the deployments D1–D7, one anchor each. Check a claim against the use
  case's `state:` and *Today*, not against the backlog; when a use case moves, the chip moves with it.
  `[owner]`
* **No `.go` file** (rule 14). `[gate: gate-architecture]`

## What the site may claim

* **Only what is true today.** A sentence that is not true of what is built is cut, or moved into
  the future tense, before the page it sits on goes live — merging to `main` publishes it.
  `[unchecked: no tool compares prose with what is built]`
* **Built is not available.** Something that exists in the repository and has not been published
  (an SDK not on its registry, a connector not on its marketplace, a client without a signed
  installer) is described as existing and not yet published, never as available.
  `[unchecked: whether a package is published lies outside the repository]`
* **What comes with convergence is said in the future tense** until `0.9.5` makes it true: the
  desktop and mobile clients with their installers and store listings, full offline operation,
  the formal accessibility statement ([`docs/roadmap.md`](../../docs/roadmap.md)). `[owner]`
* **Deliberately absent:** a price or a paid edition, load-test figures (they stay internal), a
  comparison table naming a competitor, and any maintenance or support commitment. `[owner]`
* A count that grows (the use cases the registry serves) is rounded down, so the claim stays true
  as the number moves.
  `[unchecked: whether a number is rounded down is a judgement of the sentence]`

## How to check a change

```bash
make website                              # builds it and what it depends on, as the workflow does
pnpm --filter @hubtask/website lint
pnpm --filter @hubtask/website typecheck
pnpm --filter @hubtask/website test
```
