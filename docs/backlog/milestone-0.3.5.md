# Milestone 0.3.5 — The workspace

The goal: this repository stops holding only a backend. It gains the two first-party clients the
product needs — the to-do application and the project website — plus the design system both of them
draw from, and it gains a pipeline that does not run a twelve-gate Go suite because somebody fixed
a typo in a stylesheet.

Nothing here adds a use case. That is the point: the milestone is measured by what still works
afterwards, not by what is new. `go build ./...`, `go test ./...`, `golangci-lint run` and
`docker compose -f deploy/docker/compose.yaml up` are green before every task and green after it,
and a contributor who only touches Go never installs a JavaScript toolchain.

It sits between 0.3.0 and 0.4.0 because the roadmap starts the frontend in parallel from 0.4.0
(`roadmap.md`, phase 5). The house has to exist before anybody moves in.

Every task is one pull request. The order is binding — each phase rests on the one before it.

Legend: **[L]** = best done locally with Claude Code (you see every step),
**[G]** = delegable through a GitHub issue.

The milestone was written with the frontend framework deliberately excluded — its ADR did not
exist, and nothing here was allowed to prejudge it. That reason is gone:
[ADR-0030](../adr/ADR-0030-svelte-frontend-framework.md) through
[ADR-0033](../adr/ADR-0033-shared-client-architecture.md) are accepted, and the milestone gained a
second wave, W-06–W-09, which finishes the house with the decisions in hand: the Svelte scaffold,
the token wiring, the embedded pipeline proven against the real bundle, and the dependency lint
the refined map needs.

What remains deliberately **not** in this milestone: any component in
`packages/design-system/src/` (its own work package on the frontend track, roadmap phase 5), the
sync engine and the Tauri shells (same track), the website's deployment target, and the extraction
of the generated SDK into a separately licensed repository
([ADR-0027](../adr/ADR-0027-monorepo-structure.md) defers that to before 1.0.0).

Three decisions taken while writing this backlog, so that nobody re-derives them later:

* **The ADRs are numbered 0027–0029, not 0026–0028.** ADR-0026 is *How the query DSL turns into
  SQL*, and CLAUDE.md rule 9 points at it.
* **`apps/web` is not a name in this project.** There are two clients and the short name does not
  say which. `apps/webapp` is the to-do application; `apps/website` is hubtask.eu. Both spellings
  are load-bearing everywhere they appear.
* **The token generator writes into the core, not into `dist/`.** `design-system.md` §1 sketches
  `dist/labeltokens.go`, but a Go file under `packages/` would be compiled by `go build ./...`
  while being unreachable by ADR-0027's rule that nothing there is importable from Go. The
  generated file therefore lands where `project-structure.md` §1 already put colour-token handling,
  `core/domain/model/shared/`, and `dist/` stays entirely generated and entirely ignored.

---

## W-01 — The three decisions **[L]**

*Depends on: nothing. No code may be written before it is finished.*

[ADR-0027](../adr/ADR-0027-monorepo-structure.md) (one repository for the core, the clients and the
design system), [ADR-0028](../adr/ADR-0028-embedded-web-ui.md) (the web UI ships inside the binary,
as an adapter) and [ADR-0029](../adr/ADR-0029-design-system-tokens.md) (the design system is code,
and `tokens.json` is its only origin). One pull request, because each is unreadable without the
other two.

The one that carries a surprise is ADR-0028. `security.md` §9 specifies a `Content-Security-Policy`
for the media origin and for nothing else, and `presentation/rest/Security.go` sends
`default-src 'none'` on every answer — right for an API that returns JSON, fatal for an HTML
document, which under it may load no script, no style and no font. So the policy for an HTML origin
is decided in the ADR rather than invented in a middleware six weeks later, and it constrains the
framework choice that has not been made yet: no `'unsafe-inline'`, no `'unsafe-eval'`.

**Acceptance:** the three ADRs exist in the house format, `docs/adr/README.md` lists them,
`make gate-docs` is green, and no line of implementation was written.

**Read:** `docs/adr/README.md`; ADR-0001; ADR-0004; ADR-0013; ADR-0014; ADR-0022; `security.md` §9

---

## W-02 — The workspace scaffold and the design system **[L]**

*Depends on: W-01. Every rule it follows is written down there.*

`apps/webapp`, `apps/website`, `packages/design-system` and `packages/api-client` come into
existence, as pnpm workspaces with a private root `package.json`, an `.nvmrc`, and a skeleton per
package that actually builds. No Go code moves — `core/`, `cmd/` and `db/` are untouched, and the
module path stays `github.com/Jersyfi/hubtask`.

The substance is the design system, and `docs/design/design-system.md` is binding for it the way
`project-structure.md` is binding for the Go layout. Style Dictionary v4 reads the DTCG source
natively and emits three targets: `tokens.css` with the primitives on `:root` and the semantic
layer under `[data-theme="light"]` and `[data-theme="dark"]`, `tokens.ts` for TypeScript, and the
ten label token names as Go constants with a validation helper — names only, never a colour value,
so the core keeps the vocabulary `domain-model.md` asks it to validate without learning what any of
it looks like.

`reference/foundations.html` is refactored to import the generated stylesheet and to declare
nothing of its own. It is the visual acceptance reference for everything built later, so it has to
render identically after the refactor as before it — and once it does, no colour value exists
anywhere but in `tokens.json`. IBM Plex ships as files (OFL-1.1, recorded in
`THIRD-PARTY-LICENSES.md`) because a self-hosted Hubtask must not contact a foreign domain on load.

**Acceptance:** `pnpm install --frozen-lockfile && pnpm -r build` succeeds from a clean checkout;
`go build ./...` and `go test ./...` succeed with no Node.js in `PATH`; regenerating the tokens
produces no diff against the committed Go file; a search for six-digit hex values across `apps/`
and `packages/` finds nothing outside `tokens.json` and generated output; light and dark mode in
`foundations.html` are unchanged; `project-structure.md` describes the new directories and the rule
that `apps/*` may depend on `packages/*` and never the reverse.

**Read:** `docs/design/design-system.md`; ADR-0027; ADR-0029; `project-structure.md` §1–§3;
`domain-model.md` §4

---

## W-03 — The UI adapter and the container build **[L]**

*Depends on: W-02 — there is nothing to embed until the webapp builds.*

`presentation/webui/` joins `rest`, `mcp` and `calendar` as an inbound adapter: an `embed.FS`, an
`http.Handler`, the UI at `/`, `/api/*` never shadowed, and unmatched non-API paths falling back to
`index.html` because a single-page application owns its own routes. `HUBTASK_UI_ENABLED` goes into
the existing configuration surface — not a second one — and the answer is reported through
`/meta/capabilities` under `features`, where a client can discover it.

The detail that makes the whole thing work is the smallest one: `presentation/webui/dist/index.html`
is **committed**, as a plain-text notice that no bundle was built into this binary. `//go:embed`
refuses to compile against a directory that is not there, so without that file every backend-only
contributor would need Node to build Go. Everything else under `dist/` is ignored.

The production Dockerfile becomes `ui` → `build` → `runtime`, with the Go dependency download kept
in its own layer ahead of the copy so that a stylesheet change does not re-download the module
cache. The result stays **one image** and Compose stays **two containers**; a proxy or a second
runtime service would break the sentence people decided to self-host on.

**Acceptance:** `docker build` produces one image that serves the webapp at `/` and the API at
`/api/v1`; `/api/v1/meta/health` answers in a running container; `HUBTASK_UI_ENABLED=false` gives
404 at `/` and leaves the API untouched; `docker compose up -d` still shows exactly two containers;
`go build ./...` succeeds in a container without Node; the UI responses carry the header set of
`security.md` §9 with the CSP ADR-0028 decided, and `/api` keeps `default-src 'none'`.

**Read:** ADR-0028; ADR-0014; `security.md` §9; `api-guidelines.md`; `project-structure.md` §4

---

## W-04 — A pipeline that runs where the change is **[L]**

*Depends on: W-03 — the container job cannot be written before there is a container to build.*

Every workflow triggers on every pull request; a `changes` job decides inside the jobs what runs.
Go, OpenAPI, database and deployment changes get the full Go pipeline with all twelve security
gates. An OpenAPI change additionally regenerates `packages/api-client` and fails on drift; a
design-system change regenerates all three token targets and fails if the committed Go file moved.
Node work runs only for the packages it touches. A documentation-only change gets a markdown lint
and a link check. Secret scanning, dependency review and the licence check always run, because
those two get in through any path.

The trap is `paths:` at workflow level. A required check that is skipped is not green — it is
pending, and it is pending forever, so the pull request can never merge. The filtering therefore
lives in `if:` conditions on job level, and one aggregator job `ci-required` with `if: always()`
collects every other job and becomes the **only** required status check. Which branch protection
settings that implies has to be written down for a human to apply, because a workflow cannot change
them.

**Acceptance:** a documentation-only pull request runs the docs job, the always-on jobs and nothing
else, and `ci-required` is green; a Go-only pull request runs the full gate suite; a
design-system-only pull request runs the token drift check and the Node jobs; a pull request
touching `.github/` runs everything; `main` and tags run the full pipeline unconditionally;
`ci-required` fails when any dependency failed or was cancelled and passes when they succeeded or
were skipped; `CONTRIBUTING.md` names it as the required check.

**Read:** `ci-cd.md`; ADR-0022; ADR-0015; `support-matrix.md`

---

## W-05 — Keeping the assistant oriented **[L]**

*Depends on: W-04 — the last thing written down is the map of what is now there.*

The root `CLAUDE.md` gains the top-level map, the dependency rules, which command verifies which
part of the tree, and two sentences that cost nothing to write and a great deal to discover the
hard way: `core/` must not learn about the frontend, and no colour, spacing or duration value is
written outside `tokens.json`. Nested `CLAUDE.md` files in `core/`, `presentation/`, both apps and
both packages keep the loaded context small — each says what the area is, what must not be done in
it, and how to verify a change to it.

`.github/CODEOWNERS` gets path-based ownership even with one maintainer, because it is the only
machine-readable statement of where the boundaries are. `area:` labels carry the dimension that a
personal repository cannot express as issue fields, and the issue and pull request templates start
offering them.

**Acceptance:** every nested `CLAUDE.md` fits on a screen; the root file names each verification
command against the part of the tree it covers; `gh label list` shows the eight `area:` labels; the
pull request template asks for the affected areas and whether an ADR is required; `make verify` is
green and `make generate` produces no diff.

**Read:** `CLAUDE.md`; `CONTRIBUTING.md`; `engineering-guidelines.md` §3; ADR-0027

---

## W-06 — The webapp scaffold: Svelte 5 as a plain Vite SPA **[L]**

*Depends on: W-05, and on the acceptance of ADR-0030 and ADR-0033 — both given.*

The framework-free skeleton in `apps/webapp` becomes what
[ADR-0030](../adr/ADR-0030-svelte-frontend-framework.md) decided: Svelte 5 (runes) with
TypeScript, built by Vite as a static single-page application. Deliberately **no SvelteKit** —
its inline bootstrap script cannot pass the CSP [ADR-0028](../adr/ADR-0028-embedded-web-ui.md)
fixed, and Vite's `index.html` references only external module scripts.

Three things belong to the scaffold and nothing more. First, client-side routing over the History
API — real paths, because ADR-0028's `index.html` fallback exists so deep links survive a reload.
Whether that router is a small library or a minimal in-house module is a supply-chain decision
(CLAUDE.md, "What you do not decide yourself"): the task proposes it with reasoning before
anything is installed. Second, the platform seam from
[ADR-0033](../adr/ADR-0033-shared-client-architecture.md): `src/lib/platform/` defines the
platform interface with the browser implementation, so no `isTauri` conditional ever lands in a
component. Third, the promise kept mechanical: a **CSP conformance check** in the webapp's CI lane
that fails on any inline `<script>` or `<style>` in the built bundle — the constraint ADR-0028
placed before the framework, enforced after it.

No feature work: no view, no API call beyond a health probe, no component. The committed
`presentation/webui/dist/index.html` placeholder and everything W-03 built stay untouched.

**Acceptance:** `pnpm --filter @hubtask/webapp build && lint && typecheck && test` green; the
built bundle contains no inline script and no inline style, and the check proving that fails on a
deliberately planted violation before it is trusted (the `gate-selftest` habit); `go build ./...`
and `go test ./...` succeed with no Node.js in `PATH`; the router dependency (or the decision to
write none) is recorded in the pull request; `apps/webapp/CLAUDE.md` still tells the truth.

**Read:** ADR-0030; ADR-0033; ADR-0028 (the CSP and the fallback rule); `apps/webapp/CLAUDE.md`

---

## W-07 — Design-system token wiring in the webapp **[G]**

*Depends on: W-06 — there is nothing to style until the app exists.*

The webapp starts drawing every value from the design system: `tokens.css` and `fonts.css`
imported once at the application root, `data-theme` set deliberately on the document — the
generated stylesheet has **no** `:root` fallback on purpose, so a document that forgets the
attribute looks broken at once. The scaffold follows the system preference
(`prefers-color-scheme`) until the account preference exists to override it. Where a value is
needed in script, it comes through `tokens.ts` as a custom-property reference, never as a literal
— the pattern is written into `apps/webapp/CLAUDE.md` so the second consumer copies the right
thing.

**Acceptance:** `pnpm --filter @hubtask/webapp lint` proves zero literal colour, spacing, radius
or duration values; both modes render and switch with `data-theme`; IBM Plex loads from the
bundle and the network tab shows no foreign origin; a page using heading, body and data styles
looks the same as the corresponding section of `reference/foundations.html`.

**Read:** ADR-0029; ADR-0030; `docs/design/design-system.md` §1, §3, §5;
`packages/design-system/README.md`

---

## W-08 — The embedded pipeline against the real bundle **[L]**

*Depends on: W-06 — and on W-03, which built the stage this proves.*

W-03 built the `ui` → `build` → `runtime` container stages against a skeleton. This task makes
them true for the real thing: the `ui` stage builds `packages/api-client`, then
`packages/design-system`, then the Svelte webapp, and the binary embeds the result. What was
decided on paper in ADR-0028 is now observable in a running container and gets verified there:
content-hashed assets answer `Cache-Control: public, max-age=31536000, immutable` while
`index.html` answers `no-cache`; a deep link reloaded inside the SPA returns the application, not
a 404, while an unmatched `/api` path still returns the API's own `Problem` 404; the UI responses
carry the UI policy of `security.md` §9 and the browser console shows no CSP violation.

**Acceptance:** `make gate-compose` is green; the Compose stack is still exactly two containers
and serves the Svelte app at `/`; the header pairings and the fallback behaviour above are
asserted by tests, not by hand; `HUBTASK_UI_ENABLED=false` still yields 404 at `/` with the API
untouched; a Go-only change still builds without Node.

**Read:** ADR-0028; ADR-0030; W-03 in this file; `security.md` §9; `deploy/docker/`

---

## W-09 — The workspace dependency lint **[G]**

*Depends on: W-02. Independent of W-06 — it guards the map, not the framework.*

[ADR-0033](../adr/ADR-0033-shared-client-architecture.md) refined the workspace map:
`apps/* → packages/*`, `packages/* → packages/*` acyclically, never `apps/* → apps/*`, and
nothing anywhere depends on an app. Today the whole map holds by convention; the Go side has
`gate-architecture`, the workspace has nothing. This task gives it the pnpm counterpart: a check
over the workspace manifests (and imports, where manifests cannot see them) that fails on any
edge outside the map, wired into the existing path-filtered Node lane so it runs when a manifest
or an import changes.

**Acceptance:** the check passes on the current tree; planted violations — an app importing an
app, a package importing an app, a dependency cycle between packages — each fail it
(`gate-selftest` habit again); `project-structure.md` §2 names the check the way it names
`gate-architecture`.

**Read:** ADR-0033; ADR-0027 rule 3; `project-structure.md` §2

---

## The order at a glance

```
W-01 ── W-02 ── W-03 ── W-04 ── W-05 ── W-06 ─┬─ W-07
        └─ W-09                               └─ W-08
```

The first wave is a straight chain — each task the ground the next one stands on: the decisions
before the scaffold, the scaffold before there is anything to embed, the embedding before there is
a container job to write, and the map last, once there is something to map. The second wave hangs
off it: the Svelte scaffold first, then token wiring and the container proof in either order, and
the dependency lint whenever after the workspace exists.

**Definition of Done for the milestone:** `apps/webapp`, `apps/website`,
`packages/design-system` and `packages/api-client` exist and build; the production image is still
one image and Compose still two containers, now serving a UI at `/`; `go build ./...`,
`go test ./...` and `golangci-lint run` succeed with no Node.js installed; regenerating the design
tokens produces no diff; the only required status check is `ci-required`, and a documentation-only
pull request reaches it without running a security gate; `git log --follow` still works across
every file the milestone moved. For the second wave: the webapp at `/` is the Svelte 5 SPA, its
built bundle carries no inline script or style and the check proving it has failed a planted
violation once, every value it renders comes from `tokens.json`, and the workspace dependency
map is enforced by a lint rather than by memory.
