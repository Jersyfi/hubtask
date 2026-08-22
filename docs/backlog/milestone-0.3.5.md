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

What deliberately is **not** in this milestone: the webapp's frontend framework (its own ADR, and
nothing here may prejudge it), any component in `packages/design-system/src/` (blocked by the same
decision), the website's deployment target, and the extraction of the generated SDK into a
separately licensed repository ([ADR-0027](../adr/ADR-0027-monorepo-structure.md) defers that to
before 1.0.0).

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

## The order at a glance

```
W-01 ── W-02 ── W-03 ── W-04 ── W-05
```

A straight chain, unlike every other milestone in this backlog. Each task is the ground the next
one stands on: the decisions before the scaffold, the scaffold before there is anything to embed,
the embedding before there is a container job to write, and the map last, once there is something
to map.

**Definition of Done for the milestone:** `apps/webapp`, `apps/website`,
`packages/design-system` and `packages/api-client` exist and build; the production image is still
one image and Compose still two containers, now serving a UI at `/`; `go build ./...`,
`go test ./...` and `golangci-lint run` succeed with no Node.js installed; regenerating the design
tokens produces no diff; the only required status check is `ci-required`, and a documentation-only
pull request reaches it without running a security gate; `git log --follow` still works across
every file the milestone moved.
