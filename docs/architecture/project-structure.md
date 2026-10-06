# Project Structure & Code Conventions

This is the one detailed map of the repository: where each kind of code lives, which way the
dependencies point, and the conventions that hold everywhere. The Go half follows the in-house
*Go hexagonal template* (`core/`, `presentation/`, `Port.go`, PascalCase file names)
([ADR-0001](../adr/ADR-0001-hexagonal-architecture.md)); the JavaScript half is a pnpm workspace in
the same repository ([ADR-0027](../adr/ADR-0027-monorepo-structure.md)).

---

## 1. Directory tree

```text
hubtask/
├── cmd/                            # the binaries
│   ├── server/main.go              # the composition root; roles via HUBTASK_ROLES
│   ├── migrate/                    # goose migrations (Compose one-shot, Helm pre-upgrade hook)
│   ├── hubctl/                     # the CLI: administration, import/export, sync conformance
│   └── restore-drill/              # the restore drill a release depends on (RT-9)
│
├── core/                           # technology-free: no SQL, HTTP, JSON tags, framework types
│   ├── domain/
│   │   ├── model/                  # one package per context: activity, automation, backup,
│   │   │                           # identity, importer, integration, job, jumble, lifecycle,
│   │   │                           # media, notification, privacy, suggestion, sync, view, work,
│   │   │                           # and shared (ID, Errors, HLC, Text, LabelTokens.go - §6)
│   │   ├── service/                # pure domain services: Hierarchy, Assignment, Authorization,
│   │   │                           # Completion, ContainerScopes, ItemAccess, Ordering
│   │   └── event/                  # EventType.go, Envelope.go, one file per event family
│   ├── application/
│   │   ├── service/<context>/      # the use cases, one handler per file
│   │   ├── usecase/                # Registry.go, Input.go - the descriptor every channel reads
│   │   ├── catalogue/              # Catalogue.go - every use case once, without dependencies
│   │   ├── repository/<context>/   # the repository interfaces (one Port.go per context)
│   │   ├── archive/                # the backup archive format: Manifest, Record, Writer, Reader
│   │   ├── condition/              # the values a CEL condition is evaluated against
│   │   └── shared/                 # ActorContext.go, NotFound.go
│   ├── port/<name>/Port.go         # outbound ports: ai, audit, backupstorage, clock, crypto,
│   │                               # environment, eventbus, expression, health, httpclient, i18n,
│   │                               # identityprovider, mail, persistence, queue, recurrence,
│   │                               # stepup, storage, text
│   └── shared/                     # concurrency/SafeGo.go (the only place `go` is allowed),
│                                   # correlation/, secret/ (the masking type)
│
├── presentation/                   # inbound adapters
│   ├── rest/                       # controllers, Router.go, Middleware.go, Problem.go,
│   │                               # Idempotency.go, RateLimit.go, LoadShed.go, Security.go,
│   │                               # ServerSentEvents.go, Deprecations.gen.go (generated)
│   ├── mcp/                        # McpServer.go, ToolRegistry.go (from the use case registry)
│   ├── stream/                     # the change stream's credential, frames and registry
│   ├── calendar/                   # Ics.go, CalDav.go, CalDavWrite.go
│   ├── intake/                     # MailIntake.go, MailParser.go, WebhookIntake.go
│   ├── worker/                     # Runner.go, Scheduler.go and one handler per job kind
│   ├── webui/                      # Embed.go, Handler.go, dist/index.html (§6, §7)
│   └── openapi/                    # Api.gen.go - generated server types, never edited
│
├── infrastructure/                 # outbound adapters, one package per port or concern
│   ├── postgres/                   # repositories, Pool.go, Tenant.go (the transaction wrapper),
│   │                               # Outbox.go, ChangeLog.go, Queue.go, Leader.go, AuditSink.go,
│   │                               # query/ (the query DSL compiler), sqlc/ (generated)
│   ├── storage/                    # LocalStorage.go, S3Storage.go, UploadGuard.go
│   ├── backupstorage/              # LocalStore.go, S3Store.go, SFTPStore.go, WebDAVStore.go
│   ├── mail/                       # SMTP.go, Resilient.go
│   ├── httpclient/                 # GuardedClient.go, Guard.go (SSRF, timeouts)
│   ├── eventbus/                   # OutboxBus.go, NATS.go, CloudEvent.go
│   ├── ai/                         # OpenAiCompatible.go, Ollama.go, Noop.go, prompts/*.md
│   ├── oidc/                       # Provider.go - the only package that imports go-oidc
│   ├── i18n/                       # the catalogue, negotiation and the ICU renderer
│   ├── observability/              # Otel.go, Metrics.go, Logger.go, UseCase.go
│   ├── resilience/                 # Timeouts, CircuitBreaker, Retry, LoadShedder, Bulkhead
│   ├── security/                   # token hashers, signed cursors, WebhookSigner.go
│   ├── crypto/                     # Envelope.go, Keyring.go, Password.go, Passphrase.go
│   ├── expression/                 # CEL.go
│   ├── recurrence/                 # Expander.go (RRULE), golden DST files in testdata/
│   ├── importer/                   # Csv, Trello, GoogleTasks, MicrosoftTodo
│   ├── audit/                      # HashChain.go (the sink itself is in postgres/)
│   ├── automation/                 # ActionDispatcher.go, OutboundCall.go
│   ├── webhook/                    # Deliverer.go
│   ├── awssig/                     # SigV4.go
│   └── clock/, environment/, health/, instancefile/, text/, archive/
│
├── api/
│   ├── openapi.yaml                # the single source of truth for the REST API
│   ├── openapi.json                # the same document as JSON, generated and committed (§6)
│   ├── oapi-codegen.yaml           # the server generator's configuration
│   ├── events/                     # JSON schemas of the CloudEvents (v1)
│   ├── fixtures/                   # shared test fixtures read by Go and TypeScript
│   └── LICENSE                     # the contract is extractable on its own
│
├── db/
│   ├── migrations/                 # NNNN_name.sql (goose, forward only)
│   ├── queries/                    # sqlc input (*.sql)
│   ├── schema.sql                  # the reference schema, diffed against a migrated database
│   ├── sqlc.yaml
│   └── embed.go
│
├── deploy/
│   ├── docker/                     # Dockerfile, compose.yaml, compose.dev.yaml
│   ├── observability/              # alerts/, dashboards/, runbooks/RB-Axx-*.md
│   ├── integration/                # the integration environment's manifests and scripts
│   ├── production/                 # the platform interface and the reference values
│   └── privacy/                    # empty; the privacy documents are in docs/privacy/
├── k8s/                            # the Helm chart: Chart.yaml, values.yaml, templates/
│
├── apps/                           # first-party clients (ADR-0027) - no Go code, ever
│   ├── webapp/                     # the to-do application in the browser; embedded (§7)
│   └── website/                    # the project website hubtask.eu; static, never embedded
├── packages/                       # what the clients share, and the connectors
│   ├── design-system/              # tokens/tokens.json, the CSS layer, components, workbench
│   ├── api-client/                 # types generated from api/openapi.yaml; generated output only
│   ├── sync-engine/                # the client's data seam: the ports, the only caller of fetch
│   ├── n8n-nodes-hubtask/          # the n8n community node, generated (§2.2)
│   └── zapier-app/                 # the Zapier app, generated (§2.2)
├── sdk/                            # the client SDKs, generated (§2.2)
│   ├── go/hubtask/                 # client.gen.go plus the few hand-written lines in hubtask.go
│   ├── python/hubtask/             # client.py, types.py, __init__.py; pyproject.toml beside it
│   └── typescript/                 # src/client.gen.ts; a workspace member, an island (§2.1)
├── pnpm-workspace.yaml             # apps/*, packages/* and sdk/typescript
├── package.json                    # workspace root: private, scripts and packageManager only
├── .nvmrc
│
├── locales/                        # en.json (the source), de.json, …; Embed.go
├── test/                           # suites that span packages (unit tests sit beside the code)
│   ├── architecture/               # layer rules, the `go` ban, parity, authorisation, message codes
│   ├── integration/                # Testcontainers PostgreSQL
│   ├── contract/                   # OpenAPI and event schema contracts
│   ├── security/                   # cross-tenant, SSRF, auth negatives, upload matrix, fuzz
│   ├── backup/, retention/, sync/, audit/, privacy/, resilience/, observability/, load/
│   ├── golden-archives/            # one reference archive per major version, for BK-4
│   └── dbtest/, s3test/, e2e/, evidence/   # shared harnesses and generated evidence
├── docs/                           # vision/, usecases/, architecture/, design/, adr/, audit/,
│                                   # privacy/, evidence/, backlog/, archive/, roadmap.md;
│                                   # docs/README.md says what each holds
├── tools/                          # checkdocs (gate-docs), checkpr (gate-pr), openapijson, sdkgen,
│                                   # deprecations, eventmatrix (make generate), locales,
│                                   # verifypr and cilocal (make verify-pr), licenses.md.tpl
├── build/lint-workspace-map.mjs    # the workspace map's gate (§2.1)
├── scripts/                        # gate-selftest, hubctl-e2e, smoke tests, drills, hooks/
├── third-party/licenses/           # the licence texts the binary and the bundle carry
├── .github/workflows/              # CI/CD (ci-cd.md)
├── go.mod                          # module github.com/Jersyfi/hubtask
├── Makefile
└── README.md
```

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

Forbidden, and checked twice — by `depguard` in `golangci-lint` (`.golangci.yml`) and by
`test/architecture` (a linter can be reconfigured in a pull request; a red test is visible in the
review):

* `core/**` importing `net/http` or `database/sql`; `core/domain` importing `encoding/json`.
* `infrastructure/**` importing `core/application/service` (an outbound adapter implements ports,
  it does not drive use cases) or `presentation/**` (adapters do not know each other).
* `presentation/**` importing `infrastructure/**`: an inbound adapter reaches the outbound side
  through the application layer only.
* The PostgreSQL driver (`github.com/jackc/pgx`) outside `infrastructure/postgres`, `cmd/migrate`,
  `cmd/restore-drill` and the database suites named in `.golangci.yml`. Every query goes through
  the transaction wrapper ([ADR-0010](../adr/ADR-0010-multi-tenancy.md)).
* `math/rand` anywhere; `fmt` and the driver in `infrastructure/postgres/query`
  ([ADR-0026](../adr/ADR-0026-query-dsl-sql-construction.md)).
* A bare `go` statement outside `core/shared/concurrency`.
* Business logic in `presentation/**` (the review checklist, not automated).

### 2.1 The workspace side

The JavaScript half has its own direction of dependency, and it points the same way — inwards,
towards what is shared:

```
apps/webapp  → packages/design-system, packages/sync-engine
apps/website → packages/design-system, packages/api-client (the document, at build time)
packages/*   → other packages/* only, acyclically (ADR-0033)
             sync-engine → api-client, and nothing else new
             n8n-nodes-hubtask, zapier-app → api-client (the document, at build time)
sdk/*        → nothing in the workspace, and nothing in the workspace → sdk/*
```

`apps/webapp` reaches the contract *through* the engine: `sync-engine` re-exports the types it
needs, and the engine is the only edge between `apps/webapp` and `api-client`. `apps/website` has
the other edge to `api-client`, and it reads only the **document** — `dist/openapi.json` and
`dist/events.json`, which `make api-client` copies from `api/` — at build time, to prerender the
API reference. It never imports a type and never calls the API.

`sdk/typescript` is a workspace member so that the Node lane builds, typechecks and tests it, and
an island on the map: it generates its own types from `api/openapi.yaml`, depends on no
first-party member, and no first-party member depends on it.

**The client's structure** ([ADR-0033](../adr/ADR-0033-shared-client-architecture.md)):

* `apps/webapp` is the one product UI. Every target — the browser bundle and the Tauri shells
  (ADR-0031, not built yet) — builds from it. Platform differences sit behind one seam,
  `apps/webapp/src/lib/platform/`, chosen at build time; components carry no platform
  conditionals.
* `packages/sync-engine` is framework-agnostic TypeScript with no Svelte in it. It implements the
  client half of offline sync against three ports it defines — `Transport`, `Storage`, `Clock` —
  and its suite runs headless against fakes. Svelte binds to it through a thin adapter in
  `apps/webapp`; a component never talks to storage or transport directly.
* **The engine never merges.** Merging is the server's
  ([offline-sync.md](./offline-sync.md) §4); the engine queues, pushes, applies what the server
  answers and surfaces `conflict`/`rejected` for the UI. A merge rule in the package is a defect.
* **The engine is product-agnostic.** Which paths a record makes stale, which reads the local copy
  can answer and which writes are which mutation kind are supplied by `apps/webapp/src/lib/data/`
  (`pathsFor`, `storeFor`, `mutationFor`); the engine never learns what a hub is.

Forbidden:

* **`packages/*` depending on `apps/*`.** A package that knows about an application is that
  application with an extra directory in the path.
* **`apps/*` depending on `apps/*`.** The two clients share nothing that is not a package.
* **An edge between `sdk/*` and the rest of the workspace, in either direction.** An SDK that
  imports a first-party package carries its release cadence and cannot be extracted; a first-party
  app that imports the SDK reaches past the sync engine's seam.
* **A runtime dependency in a generated SDK.** The Go client imports only `oapi-codegen/runtime`,
  the TypeScript client only `fetch`, the Python client only the standard library.
* **Any Go code under `apps/` or `packages/`.** No `.go` file is committed under either. The
  traffic runs the other way: the design system *generates* one Go file into the core (§6).
* **A JavaScript toolchain in the path of a Go build.** `go build ./...`, `go test ./...`,
  `golangci-lint run` and `make generate` work in a checkout where Node.js was never installed. A
  change that breaks this is reverted, not documented — which is why
  `presentation/webui/dist/index.html` is committed and `make tokens` is separate from
  `make generate` (§6).
* **A monorepo build orchestrator.** JavaScript builds are pnpm workspaces plus the `Makefile`;
  Nx, Turborepo or Bazel would be a second build system to keep in step with `make verify`.

One place is reached out of a member on purpose: **the catalogues under `locales/`**, which
`apps/webapp/src/lib/i18n/catalogue.ts` reads — `en.json` imported as the source, the others
through `import.meta.glob('locales/*.json')`, one lazy chunk per file and none in the initial
bundle. The source is the product's single catalogue of display text
([i18n-l10n.md](./i18n-l10n.md) §3); it sits at the root because the Go binary embeds it
(`locales/Embed.go`), and a copy under `apps/` would be a second source of truth. The lint carries
the exception as exactly `locales/<name>.json` — a static import, a dynamic one or a glob, from
that one module — so an escape towards anything else, including `locales/Embed.go`, is still
refused, and its selftest plants both.

Tooling: `build/lint-workspace-map.mjs` (`pnpm lint:workspace`), the pnpm counterpart of
`gate-architecture`. It reads the manifests and the imports — a manifest cannot see a deep or
relative import that crosses a member boundary — and fails on any edge outside the map above. CI
runs it in every Node lane, `--selftest` first, so each forbidden edge kind is proven to be caught
before the real tree is trusted.

### 2.2 Generated clients and connectors

| Where | What | Rule |
|---|---|---|
| `sdk/go/`, `sdk/python/`, `sdk/typescript/` | The client SDKs, generated from `api/openapi.yaml` by `make generate` | Each directory carries its own `LICENSE`, so that it can be extracted into a repository of its own as a move rather than a rewrite ([ADR-0057](../adr/ADR-0057-sdk-licence-and-extraction.md)). The package names (npm scope, PyPI project) and any extraction are not decided |
| `packages/api-client/` | The TypeScript types the first-party apps compile against | Generated output only (`make api-client`); nothing hand-written |
| `packages/n8n-nodes-hubtask/`, `packages/zapier-app/` | The connectors, generated by each package's `pnpm build` from the document `make api-client` writes, so a contract change changes them in the same pull request ([ADR-0058](../adr/ADR-0058-connector-packages.md)) | The workspace manifest names **no platform library** — pnpm would install it and its tree. The manifest the platform reads is generated into `dist/package.json` (`n8n-workflow` as a peer; `zapier-platform-core` as the exact dependency the Zapier CLI demands), and `dist/` is what is published. The output is held to a schema of the platform's format in the package's tests; loading it into the real platform is the check a type cannot replace |

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
| Context | `context.Context` is always the first argument; actor and tenant travel via `ActorContext` (a typed wrapper, no bare context value access in business code) |
| Time/randomness/IDs | Exclusively through the `Clock`, `RandomSource` and `IDGenerator` ports ([arc42.md](./arc42.md) §8.13) |
| Logging | `log/slog`, structured, no user content and no personal data, always with `request_id`/`trace_id` |
| Transactions | Exclusively in the application layer through the unit of work; repositories never open transactions |
| DTOs | The application layer owns its input and output types; domain objects do not leave the core and carry no JSON tags |
| Generated code | Never changed by hand. `make generate`: `presentation/openapi`, `infrastructure/postgres/sqlc`, `api/openapi.json`, `presentation/rest/Deprecations.gen.go`, the SDKs, `docs/audit/event-matrix.md`. Outside it, because they need Node.js and `make generate` must not: `core/domain/model/shared/LabelTokens.go` (`make tokens`) and `packages/api-client` (`make api-client`) |
| Tests | Domain = table tests without mocks; application = fakes of the ports; infrastructure = Testcontainers ([engineering-guidelines.md](./engineering-guidelines.md) §1) |
| Comments | Explain the *why*; public types and functions carry a godoc comment; a decision's argument belongs in an ADR, cited by number |
| CLI (`hubctl`) | Every command keeps `--json` pipeable, renders errors through the message-code catalogue, prints a secret once with its warning on stderr, and follows a `202` to completion through `/jobs/{id}` |
| Jobs in a client | A client watches a job through the one job store in `apps/webapp/src/lib/data/` (`jobs.svelte.ts`), which polls `/jobs/{id}` with a backoff and stops at a terminal status; no screen polls a job itself |

---

## 4. Split into deployment units

One module, one image, several roles
([ADR-0002](../adr/ADR-0002-modular-monolith.md), [ADR-0014](../adr/ADR-0014-single-image-multi-role.md)).
The automation context is cut so that it can run as its own process without a code change:

| Role | Starts | Communication |
|---|---|---|
| `api` | REST, MCP, the change stream, ICS/CalDAV, intake, the web UI | Reads/writes the database |
| `worker` | Outbox dispatcher, webhook delivery, mail, media reconciliation, indexing, the job kinds | Database queue |
| `scheduler` | Reminders, occurrences, retention | Database queue + advisory lock |
| `automation` | The rule engine | Consumes events, calls use cases in-process |

The loops that make `worker` and `scheduler` real live in `presentation/worker/`, beside `rest` and
`mcp`. A job arriving and a handler running differs from a request arriving and a handler running
in who asked, not in what the layer does with it — so the queue is an inbound adapter, and like
every inbound adapter it reaches the outbound side through the application layer only.

A genuine service split would mean only this: its own `cmd/automation/main.go`, and the in-process
use case call swapped for an HTTP client behind the same interface.

---

## 5. Deviations from the template

The template is a starting point, not a constraint. What differs from it:

1. `go.mod` is `github.com/Jersyfi/hubtask`, and the module path stays that.
2. The template's example files are gone.
3. `core/port/environment/Port.go` carries the `HUBTASK_*` configuration surface
   ([arc42.md](./arc42.md) §7.4).
4. **CI/CD runs on GitHub Actions, not GitLab** ([ADR-0022](../adr/ADR-0022-github-platform.md)).
   Every gate is a `make` target, and the workflows under `.github/workflows/` only orchestrate —
   see [ci-cd.md](./ci-cd.md).
5. `k8s/` is the project's Helm chart (roles, probes, HPA, the migration hook).
6. Repository configuration (branch protection, environments, labels, required checks) is set in
   the GitHub UI and documented in [ci-cd.md](./ci-cd.md) §4. There is no bootstrap script.

---

## 6. Generated files that are committed

Generated output is not committed, except the files below, each for a reason that is about a build
that lacks one half of the toolchain:

| File | Produced by | Why it is committed |
|---|---|---|
| `presentation/webui/dist/index.html` | a placeholder, replaced by the container build | `//go:embed all:dist` refuses to compile against a directory that does not exist, so without it `go build ./...` would need a frontend build ([ADR-0028](../adr/ADR-0028-embedded-web-ui.md)) |
| `core/domain/model/shared/LabelTokens.go` | `make tokens`, from `packages/design-system/tokens/tokens.json` | the domain validates a `colorToken` against it; committing it keeps `go build ./...` working without Node and turns a drift between the design system and the domain into a diff ([ADR-0029](../adr/ADR-0029-design-system-tokens.md)) |
| `api/openapi.json` | `make generate`, through `tools/openapijson` | the website's reference and the SDK generators read the contract as JSON and ship no YAML parser, and the Node lanes that build them have no Go |
| `sdk/typescript/src/client.gen.ts`, `sdk/python/hubtask/client.py`, `types.py`, `sdk/go/hubtask/client.gen.go` | `make generate`, through `tools/sdkgen` and `oapi-codegen` | the generator is Go, and the lanes that typecheck and test its output have none |
| `presentation/openapi/Api.gen.go`, `infrastructure/postgres/sqlc/`, `presentation/rest/Deprecations.gen.go`, `docs/audit/event-matrix.md` | `make generate` | ordinary Go-side output, committed so that a checkout builds without the generators |

**None of them may be edited by hand.** CI regenerates them and fails on any difference.
`LabelTokens.go` holds the *names* of the ten label colours and never a colour value: the core stays
colour-blind while sharing one vocabulary with the frontend, which is what
[domain-model.md](./domain-model.md) §3.5 asks for when a label stores a token instead of a hex.

Everything else the workspace produces — `packages/design-system/dist/`,
`packages/api-client/dist/`, `sdk/typescript/dist/`, `apps/*/dist/` — is ignored, and reproducible
from the source plus the pnpm lockfile.

---

## 7. The embedded web UI

`presentation/webui` serves the built `apps/webapp` bundle from the binary
([ADR-0028](../adr/ADR-0028-embedded-web-ui.md)). It is an inbound adapter beside `rest` and `mcp`;
serving bytes is not a use case, so nothing is registered for it, and `core/` does not know it
exists.

| Topic | Rule |
|---|---|
| Routing | The UI is served at `/`. `/api/*` always wins: a request under `/api` that matches no route gets the API's own 404 with a problem body, never HTML. Any other unmatched path falls back to `index.html`, so a deep link survives a reload. `/healthz`, `/readyz` and the ops listener are untouched |
| Caching | Content-hashed assets `Cache-Control: public, max-age=31536000, immutable`; `index.html` `no-cache` |
| Security headers | The UI routes have their own content security policy, every source `'self'` except the media origin; it lives in [security.md](./security.md) §9 |
| Switching it off | `HUBTASK_UI_ENABLED=false` makes `/` answer 404 and leaves the API untouched; the state is announced under `features` in `/meta/capabilities` |
| The image | The container build has a `ui` stage before the Go stage (`deploy/docker/Dockerfile`: `ui` → `build` → `runtime`), so the result is still one image and Compose is still two containers |
| The website | `apps/website` is never embedded; it builds to static files and deploys on its own |
