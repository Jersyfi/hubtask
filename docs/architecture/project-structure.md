# Project Structure & Code Conventions

The one map of the repository: where each kind of code lives, which way the dependencies point, and
the conventions that hold everywhere. The Go half follows the in-house *Go hexagonal template*
(`core/`, `presentation/`, `Port.go`, PascalCase file names)
([ADR-0001](../adr/ADR-0001-hexagonal-architecture.md)); the JavaScript half is a pnpm workspace in
the same repository ([ADR-0027](../adr/ADR-0027-monorepo-structure.md)).

---

## 1. Directory tree

Where a kind of code goes; the files inside are what `ls` shows.

```text
hubtask/
├── cmd/                       # the binaries: server (main.go, the composition root; roles via
│                              # HUBTASK_ROLES), migrate (goose), hubctl (the CLI), restore-drill (RT-9)
├── core/                      # technology-free: no SQL, HTTP, JSON tags, framework types
│   ├── domain/
│   │   ├── model/<context>/   # one package per context; shared/ holds ID, Errors, HLC, Text,
│   │   │                      # LabelTokens.go (§6); view/ holds the query grammar
│   │   ├── service/           # pure domain services (Hierarchy, Assignment, Authorization, …)
│   │   └── event/             # EventType.go, Envelope.go, one file per event family
│   ├── application/
│   │   ├── service/<context>/ # the use cases, one handler per file
│   │   ├── usecase/           # Registry.go, Input.go — the descriptor every channel reads
│   │   ├── catalogue/         # Catalogue.go — every use case once, without dependencies
│   │   ├── repository/<ctx>/  # the repository interfaces (one Port.go per context)
│   │   ├── archive/           # the backup archive format
│   │   ├── condition/         # the values a CEL condition is evaluated against
│   │   └── shared/            # ActorContext.go, NotFound.go
│   ├── port/<name>/Port.go    # the outbound ports
│   └── shared/                # concurrency/SafeGo.go (the only place `go` is allowed),
│                              # correlation/, secret/ (the masking type)
├── presentation/              # inbound adapters: rest, mcp, stream, calendar, intake,
│                              # worker (the job loops, §4), webui (§7), openapi (generated)
├── infrastructure/            # outbound adapters, one package per port or concern; postgres/
│                              # holds the transaction wrapper (Tenant.go), outbox, queue, the
│                              # query DSL compiler and sqlc/ (generated); oidc/ is the only
│                              # importer of go-oidc; httpclient/ is GuardedClient
├── api/                       # openapi.yaml (the source of truth), openapi.json (generated, §6),
│                              # events/ (CloudEvent schemas), fixtures/ (shared by Go and
│                              # TypeScript), LICENSE (the contract is extractable)
├── db/                        # migrations/ (goose, forward only), queries/ (sqlc input),
│                              # schema.sql (the reference schema, regenerated from the
│                              # migrations and diffed against a migrated database)
├── deploy/                    # docker/, observability/ (alerts, dashboards, runbooks),
│                              # integration/, production/
├── k8s/                       # the Helm chart
├── apps/                      # first-party clients (ADR-0027) — no Go code, ever
│   ├── webapp/                # the product UI; embedded (§7)
│   └── website/               # hubtask.eu; static, never embedded
├── packages/                  # design-system (tokens/tokens.json, CSS, components, workbench),
│                              # api-client (generated only), sync-engine (the client's data
│                              # seam, the only caller of fetch), n8n-nodes-hubtask and
│                              # zapier-app (generated, §2.2)
├── sdk/                       # go/, python/, typescript/ — generated client SDKs (§2.2)
├── locales/                   # en.json (the source), the translations, Embed.go
├── test/                      # suites that span packages; unit tests sit beside the code
├── docs/                      # docs/README.md says what each directory holds
├── tools/                     # checkdocs, checkpr, generators, verifypr — the gates' own code
├── build/                     # lint-workspace-map.mjs (§2.1)
├── scripts/                   # gate-selftest, hubctl-e2e, smoke tests, drills, hooks
├── third-party/licenses/      # the licence texts the binary and the bundle carry
└── .github/workflows/         # CI/CD (ci-cd.md)
```

The workspace root holds `pnpm-workspace.yaml` (`apps/*`, `packages/*`, `sdk/typescript`), a
`package.json` with scripts and `packageManager` only, `.nvmrc`, `go.mod`
(`github.com/Jersyfi/hubtask`) and the `Makefile`.

---

## 2. Dependency rules (enforced in CI)

```
cmd              → anything
presentation     → core (any layer); never infrastructure or cmd
infrastructure   → core/port, core/domain, core/shared, and core/application's repository
                   interfaces and shared types; never core/application/service, presentation, cmd
core/application → core/domain, core/port, core/shared
core/domain      → core/domain, core/port (pure ports such as Clock), core/shared;
                   never core/application
core/port        → core/domain, core/shared
core/**          → the standard library only; no third-party library
```

Forbidden, and checked twice — by `depguard` (`.golangci.yml`) and by `test/architecture`, because a
linter can be reconfigured in a pull request and a red test is visible in review:

* `core/**` importing `net/http` or `database/sql`; `core/domain` importing `encoding/json`.
* `infrastructure/**` importing `core/application/service` (an outbound adapter implements ports,
  it does not drive use cases) or `presentation/**` (adapters do not know each other).
* `presentation/**` importing `infrastructure/**`: an inbound adapter reaches the outbound side
  through the application layer only.
* The PostgreSQL driver (`github.com/jackc/pgx`) outside `infrastructure/postgres`, `cmd/migrate`,
  `cmd/restore-drill` and the database suites named in `.golangci.yml`; every query goes through
  the transaction wrapper ([ADR-0010](../adr/ADR-0010-multi-tenancy.md)).
* `math/rand` anywhere; `fmt` and the driver in `infrastructure/postgres/query`
  ([ADR-0026](../adr/ADR-0026-query-dsl-sql-construction.md)).
* A bare `go` statement outside `core/shared/concurrency`.
* Business logic in `presentation/**` (the review checklist, not automated).

### 2.1 The workspace side

The JavaScript half points inwards too, towards what is shared:

```
apps/webapp  → packages/design-system, packages/sync-engine
apps/website → packages/design-system, packages/api-client (the document, at build time)
packages/*   → other packages/* only, acyclically (ADR-0033)
             sync-engine → api-client, and nothing else new
             n8n-nodes-hubtask, zapier-app → api-client (the document, at build time)
sdk/*        → nothing in the workspace, and nothing in the workspace → sdk/*
```

`apps/webapp` reaches the contract only *through* the engine, which re-exports the types it needs.
`apps/website` reads only the **document** — `dist/openapi.json` and `dist/events.json`, copied by
`make api-client` — at build time to prerender the API reference; it never imports a type or calls
the API. `sdk/typescript` is a workspace member so the Node lane builds and tests it, and an island:
it generates its own types and has no edge to any first-party member.

**The client's structure** ([ADR-0033](../adr/ADR-0033-shared-client-architecture.md)):

* `apps/webapp` is the one product UI; every target (the browser bundle and the Tauri shells of
  ADR-0031, not built yet) builds from it. Platform differences sit behind one seam,
  `apps/webapp/src/lib/platform/`, chosen at build time; components carry no platform conditionals.
* `packages/sync-engine` is framework-agnostic TypeScript implementing the client half of offline
  sync against its own ports — `Transport`, `Storage`, `Clock` — tested headless against fakes.
  Svelte binds to it through a thin adapter in `apps/webapp`; a component never talks to storage or
  transport directly.
* **The engine never merges.** Merging is the server's ([offline-sync.md](./offline-sync.md) §4);
  the engine queues, pushes, applies the server's answer and surfaces `conflict`/`rejected`. A merge
  rule in the package is a defect.
* **The engine is product-agnostic.** Which paths a record makes stale, which reads the local copy
  answers and which writes are which mutation kind come from `apps/webapp/src/lib/data/`
  (`pathsFor`, `storeFor`, `mutationFor`); the engine never learns what a hub is.

Forbidden:

* **`packages/*` depending on `apps/*`**, and **`apps/*` depending on `apps/*`** — the two clients
  share nothing that is not a package.
* **An edge between `sdk/*` and the rest of the workspace, either way**: an SDK importing a
  first-party package could not be extracted, an app importing the SDK bypasses the engine's seam.
* **A runtime dependency in a generated SDK**: Go imports only `oapi-codegen/runtime`, TypeScript
  only `fetch`, Python only the standard library.
* **Any Go code under `apps/` or `packages/`.** The traffic runs the other way: the design system
  *generates* one Go file into the core (§6).
* **A JavaScript toolchain in the path of a Go build.** `go build ./...`, `go test ./...`,
  `golangci-lint run` and `make generate` work without Node.js; a change that breaks this is
  reverted. Hence the committed `presentation/webui/dist/index.html` and `make tokens` apart from
  `make generate` (§6).
* **A monorepo build orchestrator** (Nx, Turborepo, Bazel): pnpm workspaces plus the `Makefile`.

One reach out of a member is deliberate: **the catalogues under `locales/`**, which
`apps/webapp/src/lib/i18n/catalogue.ts` reads — `en.json` imported as the source, the others through
`import.meta.glob('locales/*.json')`, one lazy chunk each, none in the initial bundle. The source is
the product's single catalogue of display text ([i18n-l10n.md](./i18n-l10n.md) §3) and sits at the
root because the Go binary embeds it. The lint allows exactly `locales/<name>.json` from that one
module, so anything else — `locales/Embed.go` included — is still refused, and its selftest plants
both.

Tooling: `build/lint-workspace-map.mjs` (`pnpm lint:workspace`), the pnpm counterpart of
`gate-architecture`. It reads manifests and imports — a manifest cannot see a deep or relative
import across a member boundary — and fails on any edge outside the map. CI runs it in every Node
lane, `--selftest` first.

### 2.2 Generated clients and connectors

| Where | What | Rule |
|---|---|---|
| `sdk/go/`, `sdk/python/`, `sdk/typescript/` | The client SDKs, generated from `api/openapi.yaml` by `make generate` | Each carries its own `LICENSE` so it can be extracted as a move, not a rewrite ([ADR-0057](../adr/ADR-0057-sdk-licence-and-extraction.md)). Package names and any extraction are not decided |
| `packages/api-client/` | The TypeScript types the first-party apps compile against | Generated output only (`make api-client`) |
| `packages/n8n-nodes-hubtask/`, `packages/zapier-app/` | The connectors, generated by each package's `pnpm build` from the document `make api-client` writes, so a contract change changes them in the same pull request ([ADR-0058](../adr/ADR-0058-connector-packages.md)) | The workspace manifest names **no platform library**; the manifest the platform reads is generated into `dist/package.json` (`n8n-workflow` as a peer, `zapier-platform-core` as the exact dependency the Zapier CLI demands), and `dist/` is published. Tests hold the output to a schema of the platform's format; loading it into the real platform is the check a type cannot replace |

Publishing a connector or an SDK to a registry is an account and a review outside this repository.

---

## 3. Conventions

| Topic | Rule |
|---|---|
| File names | PascalCase as in the template (`Item.go`, `CreateWorkItem.go`); ports are always called `Port.go` |
| Package names | Short, lower case, no underscores (`work`, `backupstorage`) |
| One use case | One file, one `Command`/`Query` struct, one handler with `Execute(ctx, …) (result, error)`, and a `Descriptor()` the registry, the catalogue and every channel read |
| Constructors | `NewX(...) (X, error)` — invariants are checked in the constructor, never afterwards |
| Errors | Typed values in `core/domain/model/shared/Errors.go`, usable with `errors.Is/As`, wrapped with `%w` |
| Context | `context.Context` is always the first argument; actor and tenant travel in `ActorContext`, never as bare context values in business code |
| Time/randomness/IDs | Only through the `Clock`, `RandomSource` and `IDGenerator` ports ([arc42.md](./arc42.md) §8.13) |
| Logging | `log/slog`, structured, no user content or personal data, always with `request_id`/`trace_id` |
| Transactions | Only in the application layer through the unit of work; repositories never open transactions |
| DTOs | The application layer owns its input and output types; domain objects stay in the core and carry no JSON tags |
| Generated code | Never changed by hand; the list is §6 |
| Tests | Domain = table tests without mocks; application = fakes of the ports; infrastructure = Testcontainers ([engineering-guidelines.md](./engineering-guidelines.md) §1) |
| Comments | Explain the *why*; public types and functions carry a godoc comment; a decision's argument belongs in an ADR, cited by number |
| CLI (`hubctl`) | Every command keeps `--json` pipeable, renders errors through the message-code catalogue, prints a secret once with its warning on stderr, and follows a `202` to completion through `/jobs/{id}` |
| Jobs in a client | Watched only through the one job store, `apps/webapp/src/lib/data/jobs.svelte.ts`, which polls `/jobs/{id}` with a backoff and stops at a terminal status; no screen polls a job itself |

---

## 4. Split into deployment units

One module, one image, several roles
([ADR-0002](../adr/ADR-0002-modular-monolith.md), [ADR-0014](../adr/ADR-0014-single-image-multi-role.md));
what each role runs is [arc42.md](./arc42.md) §7.1. `worker` and `scheduler` communicate through the
database queue (the scheduler under an advisory lock); `automation` consumes events and calls use
cases in-process. The automation context is cut so that it can run as its own process without a
code change: a genuine split would only add `cmd/automation/main.go` and swap the in-process use
case call for an HTTP client behind the same interface.

The job loops live in `presentation/worker/`, beside `rest` and `mcp`: a job differs from a request
in who asked, not in what the layer does with it, so the queue is an inbound adapter and reaches the
outbound side through the application layer only.

---

## 5. Deviations from the template

The template is a starting point, not a constraint. What differs from it:

1. `go.mod` is `github.com/Jersyfi/hubtask`, and the module path stays that.
2. The template's example files are gone.
3. `core/port/environment/Port.go` carries the `HUBTASK_*` configuration surface
   ([arc42.md](./arc42.md) §7.4).
4. **CI/CD runs on GitHub Actions, not GitLab** ([ADR-0022](../adr/ADR-0022-github-platform.md));
   every gate is a `make` target, the workflows only orchestrate ([ci-cd.md](./ci-cd.md)).
5. `k8s/` is the project's Helm chart (roles, probes, HPA, the migration hook).
6. Repository configuration (branch protection, environments, labels, required checks) is set in
   the GitHub UI and documented in [ci-cd.md](./ci-cd.md) §4; there is no bootstrap script.

---

## 6. Generated files that are committed

Generated output is not committed, except these — each because a build lacking one half of the
toolchain needs it:

| File | Produced by | Why it is committed |
|---|---|---|
| `presentation/webui/dist/index.html` | a placeholder, replaced by the container build | `//go:embed all:dist` does not compile against a missing directory, so `go build ./...` would need a frontend build ([ADR-0028](../adr/ADR-0028-embedded-web-ui.md)) |
| `core/domain/model/shared/LabelTokens.go` | `make tokens`, from `packages/design-system/tokens/tokens.json` | the domain validates a `colorToken` against it; committed, it keeps `go build` free of Node and turns drift between design system and domain into a diff ([ADR-0029](../adr/ADR-0029-design-system-tokens.md)) |
| `api/openapi.json` | `make generate`, through `tools/openapijson` | the website's reference and the SDK generators read JSON and ship no YAML parser, and their Node lanes have no Go |
| `sdk/typescript/src/client.gen.ts`, `sdk/python/hubtask/client.py`, `types.py`, `sdk/go/hubtask/client.gen.go` | `make generate`, through `tools/sdkgen` and `oapi-codegen` | the generator is Go, and the lanes that test its output have none |
| `presentation/openapi/Api.gen.go`, `infrastructure/postgres/sqlc/`, `presentation/rest/Deprecations.gen.go`, `docs/audit/event-matrix.md` | `make generate` | ordinary Go-side output, so a checkout builds without the generators |

**None of them may be edited by hand.** CI regenerates them and fails on any difference.
`LabelTokens.go` and `packages/api-client` (`make api-client`) stay outside `make generate` because
they need Node.js. `LabelTokens.go` holds the *names* of the ten label colours, never a value: the
core stays colour-blind while sharing one vocabulary with the frontend
([domain-model.md](./domain-model.md) §3.5).

Everything else the workspace builds (`packages/*/dist/`, `sdk/typescript/dist/`, `apps/*/dist/`) is
ignored and reproducible from the source and the pnpm lockfile.

---

## 7. The embedded web UI

`presentation/webui` serves the built `apps/webapp` bundle from the binary
([ADR-0028](../adr/ADR-0028-embedded-web-ui.md)): an inbound adapter beside `rest` and `mcp`. Serving
bytes is not a use case, so nothing is registered for it, and `core/` does not know it exists.

| Topic | Rule |
|---|---|
| Routing | The UI is served at `/`. `/api/*` always wins: an unmatched `/api` path gets the API's own 404 problem, never HTML. Any other unmatched path falls back to `index.html`, so a deep link survives a reload. `/healthz`, `/readyz` and the ops listener are untouched |
| Caching | Content-hashed assets `Cache-Control: public, max-age=31536000, immutable`; `index.html` `no-cache` |
| Security headers | The UI routes have their own content security policy, every source `'self'` except the media origin ([security.md](./security.md) §9) |
| Switching it off | `HUBTASK_UI_ENABLED=false` makes `/` answer 404 and leaves the API untouched; announced under `features` in `/meta/capabilities` |
| The image | A `ui` stage before the Go stage (`deploy/docker/Dockerfile`: `ui` → `build` → `runtime`), so it is still one image and Compose still two containers |
| The website | `apps/website` is never embedded; it builds to static files and deploys on its own |
