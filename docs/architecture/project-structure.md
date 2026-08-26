# Project Structure & Code Conventions

The basis is the in-house *Go hexagonal template* (`core/`, `presentation/`, `Port.go`, PascalCase
file names). The structure is **kept** and only extended with directories that hold adapters,
contexts, and operational artefacts.

---

## 1. Directory tree

```text
hubtask/
├── cmd/
│   ├── server/                     # the main binary; roles via HUBTASK_ROLES
│   │   └── main.go                 # dependency injection, composition root
│   ├── migrate/main.go             # goose migrations (K8s pre-upgrade job)
│   └── hubctl/main.go              # CLI: admin, import/export, dogfooding client
│
├── core/                           # technology-free — no SQL, HTTP, JSON, framework
│   ├── domain/
│   │   ├── model/
│   │   │   ├── identity/           # Tenant.go, Account.go, Membership.go, Group.go, Role.go
│   │   │   ├── work/               # Container.go, WorkItem.go, Bucket.go, Label.go,
│   │   │   │                       # Comment.go, Cover.go, OrderKey.go, ItemType.go,
│   │   │   │                       # CapabilityProfile.go, CustomField.go
│   │   │   ├── scheduling/         # DueDate.go, Reminder.go, RecurrenceRule.go, Occurrence.go
│   │   │   ├── automation/         # Rule.go, Trigger.go, Condition.go, Action.go, RuleRun.go
│   │   │   ├── view/               # SavedView.go, QuerySpec.go, GroupingSpec.go, Cursor.go
│   │   │   ├── jumble/             # JumbleEntry.go, IntakeChannel.go
│   │   │   ├── template/           # Template.go, TemplateNode.go
│   │   │   ├── integration/        # WebhookSubscription.go, Delivery.go, CalendarFeed.go
│   │   │   ├── media/              # MediaObject.go
│   │   │   ├── audit/              # ActivityEntry.go, AuditLog.go
│   │   │   └── shared/             # ID.go, Locale.go, TimeZone.go, ColorToken.go, Errors.go,
│   │   │                           # LabelTokens.go (generated - see §6)
│   │   ├── service/                # pure domain services
│   │   │   ├── Hierarchy.go        # permitted parents/children, depth, move, cycle check
│   │   │   ├── Capabilities.go     # evaluation of the capability profiles
│   │   │   ├── Completion.go       # roll-up policy
│   │   │   ├── Assignment.go       # strategies FIXED/RANDOM/ROUND_ROBIN/LEAST_LOADED
│   │   │   ├── Recurrence.go       # RRULE evaluation (a port for calendar arithmetic)
│   │   │   ├── Authorization.go    # role/scope inheritance
│   │   │   └── Ordering.go         # rank keys for drag and drop
│   │   └── event/                  # Events.go, EventType.go, Envelope.go
│   │
│   ├── application/
│   │   ├── service/                # use cases, one handler per file
│   │   │   ├── work/               # CreateWorkItem.go, UpdateWorkItem.go, MoveWorkItem.go, …
│   │   │   ├── scheduling/
│   │   │   ├── automation/
│   │   │   ├── view/
│   │   │   ├── jumble/
│   │   │   ├── identity/
│   │   │   ├── integration/
│   │   │   └── lifecycle/          # trash/archive/retention
│   │   ├── archive/                # the backup archive format: Manifest, Record, Writer, Reader (E-04)
│   │   ├── repository/             # a Port.go per context (repository interfaces)
│   │   ├── usecase/                # Registry.go — the catalogue of all use cases (REST/MCP/automation)
│   │   └── shared/                 # Transaction.go, ActorContext.go, Result.go, Pagination.go
│   │
│   ├── port/                       # outbound ports
│   │   ├── api/Port.go             # inbound contracts (implemented/consumed by presentation)
│   │   ├── environment/Port.go     # configuration (from the template, extended)
│   │   ├── persistence/Port.go     # UnitOfWork, Snapshot, TenantScope
│   │   ├── storage/Port.go         # object storage (S3/local)
│   │   ├── mail/Port.go            # SMTP sending, IMAP intake
│   │   ├── notification/Port.go    # channels (mail, webhook, push)
│   │   ├── eventbus/Port.go        # publish/subscribe (outbox, NATS)
│   │   ├── httpclient/Port.go      # outbound calls with SSRF protection
│   │   ├── queue/Port.go           # jobs, scheduling, leader election
│   │   ├── ai/Port.go              # completion, embedding, classification
│   │   ├── search/Port.go          # indexing, search
│   │   ├── identityprovider/Port.go# OIDC
│   │   ├── clock/Port.go           # Clock, IDGenerator, RandomSource
│   │   ├── i18n/Port.go            # message catalogue, formatting
│   │   ├── metrics/Port.go         # (from the template)
│   │   └── logging/Port.go         # (from the template)
│   │
│   ├── health/Port.go              # DependencyProbe, HealthReport (self-diagnosis)
│   ├── backupstorage/Port.go       # target abstraction: put/get/list/delete/stat
│   ├── audit/Port.go               # AuditSink, ChainVerifier, SiemExporter
│   └── shared/                     # HelperFunctions.go (from the template), validation utilities,
│                                   # concurrency/SafeGo.go (the only place `go` is allowed),
│                                   # secret/Secret.go (the masking type)
│
├── presentation/                   # inbound adapters
│   ├── rest/                       # RestController.go, Router.go, Mapper.go, Problem.go,
│   │                               # Middleware.go (auth, tenant, locale, rate limit, idempotency)
│   ├── mcp/                        # McpServer.go, ToolRegistry.go (generated from usecase.Registry)
│   ├── sse/                        # StreamController.go
│   ├── calendar/                   # IcsController.go, CalDavController.go
│   ├── intake/                     # MailIntake.go, WebhookIntake.go
│   ├── worker/                     # Runner.go, Scheduler.go — the queue as an inbound channel
│   ├── admin/                      # AdminController.go (tenants, quotas, metering)
│   ├── webui/                      # embed.go - the built apps/webapp bundle, served at / (ADR-0028)
│   └── openapi/                    # generated server types (never edit by hand)
│
├── infrastructure/                 # outbound adapters
│   ├── postgres/                   # repositories, sqlc output, Outbox.go, ChangeLog.go, AuditSink.go,
│   │                               # Queue.go, Tenant.go (RLS)
│   ├── storage/                    # S3Storage.go, LocalStorage.go
│   ├── mail/                       # SmtpSender.go, ImapPoller.go
│   ├── httpclient/                 # GuardedClient.go (SSRF, timeouts, retry)
│   ├── eventbus/                   # OutboxBus.go, NatsBus.go
│   ├── ai/                         # OpenAiCompatible.go, Ollama.go, NoopAi.go
│   ├── identityprovider/           # OidcProvider.go, LocalProvider.go
│   ├── search/                     # PostgresSearch.go, VectorSearch.go
│   ├── i18n/                       # IcuCatalog.go, embed.go
│   ├── observability/              # Otel.go, Metrics.go, Logger.go, Redaction.go,
│   │                               # HealthReporter.go, DegradationRegistry.go
│   ├── resilience/                 # Timeouts.go, CircuitBreaker.go, Retry.go, LoadShedder.go, Bulkhead.go
│   ├── security/                   # Argon2.go, TokenHasher.go, Envelope.go, HmacSigner.go, Headers.go
│   ├── backupstorage/              # LocalTarget.go, S3Target.go, SftpTarget.go, FtpTarget.go,
│   │                               # WebdavTarget.go, SmbTarget.go, RcloneTarget.go, HttpPutTarget.go
│   ├── archive/                    # Writer.go, Reader.go, Manifest.go, Encryption.go, Migrate.go
│   ├── audit/                      # HashChain.go, SiemExporter.go (the sink itself is in
│   │                               # postgres/, where rule 3 keeps the driver)
│   └── automation/                 # CelEvaluator.go, ActionDispatcher.go
│
├── api/
│   ├── openapi.yaml                # the single source of truth for the REST API
│   ├── events/                     # JSON schemas of the CloudEvents (v1)
│   └── mcp/manifest.json           # the generated MCP tool manifest
│
├── db/
│   ├── migrations/                 # 0001_init.sql, … (goose, forward only)
│   ├── queries/                    # sqlc input (*.sql)
│   ├── schema.sql                  # generated reference of the target schema
│   └── sqlc.yaml
│
├── deploy/
│   ├── docker/                     # Dockerfile, compose.yaml, compose.dev.yaml
│   ├── observability/              # dashboards/, alerts/prometheus-rules.yaml, runbooks/RB-xx.md
│   ├── privacy/                    # DPA template, sub-processor list, TOM description
│   └── k8s/ → k8s/                 # Helm chart per the template (Chart.yaml, values*.yaml)
│
├── apps/                           # first-party clients (ADR-0027) - no Go code, ever
│   ├── webapp/                     # the to-do application in the browser; embedded (ADR-0028)
│   └── website/                    # the project website hubtask.eu; static, never embedded
├── packages/                       # what the clients share (ADR-0027)
│   ├── design-system/              # tokens/tokens.json + the CSS layer (ADR-0029)
│   └── api-client/                 # generated from api/openapi.yaml; generated output only
├── pnpm-workspace.yaml             # apps/* and packages/*
├── package.json                    # workspace root: private, scripts and packageManager only
├── .nvmrc
│
├── locales/                        # en.json (source), de.json, … (ICU MessageFormat)
├── test/
│   ├── integration/                # Testcontainers PostgreSQL
│   ├── contract/                   # OpenAPI and event schema contracts
│   ├── architecture/               # import rules, layer boundaries, the `go` ban, mandatory authorisation
│   ├── security/                   # cross-tenant suite, SSRF suite, auth negative tests, upload matrix, fuzz
│   ├── backup/                     # BK-1…BK-10 per target adapter (test container)
│   ├── retention/                  # RE-1…RE-9 including time zone and DST boundaries
│   ├── sync/                       # SY-1…SY-12 (concurrency, clock skew, full sync)
│   ├── audit/                      # AU-1…AU-7 (chain, grants, freedom from content)
│   ├── golden-archives/            # one reference archive per major version, for BK-4
│   ├── resilience/                 # RT-1…RT-12 (dependency failure, process death, overload, chaos)
│   └── fixtures/
├── docs/                           # arc42, ADRs, roadmap (this repository)
├── tools/                          # checkdocs/ (make gate-docs), licenses.md.tpl (make licenses)
├── .github/workflows/              # CI/CD (ADR-0022, docs/architecture/ci-cd.md)
├── go.mod                          # module github.com/Jersyfi/hubtask
├── Makefile
└── README.md
```

---

## 2. Dependency rules (enforced in CI)

```
cmd → presentation, infrastructure, core
presentation → core/application, core/port, core/domain/model (for DTO mapping only)
infrastructure → core/port, core/domain/model
core/application → core/domain, core/port
core/domain → core/domain, core/port (pure ports only, such as Clock/ID)
core/port → nothing except core/domain/model/shared
```

Forbidden, and checked by test (`test/architecture`):
* `core/**` importing `net/http`, `database/sql`, `github.com/jackc/*`, or putting `encoding/json` tags on domain types.
* `presentation/**` containing business logic (caught by the review checklist, not automated).
* `infrastructure/**` importing `core/application/service`.
* Cyclic context dependencies between `domain/model/*` packages.

Tooling: `go-arch-lint` or `depguard` (golangci-lint) with an explicit allowlist.

### 2.1 The workspace side

The JavaScript half has its own direction of dependency, and it points the same way — inwards,
towards what is shared:

```
apps/webapp  → packages/design-system, packages/api-client
apps/website → packages/design-system
packages/*   → other packages/* only, acyclically (ADR-0033)
```

Forbidden:

* **`packages/*` depending on `apps/*`.** A package that knows about an application is not a
  shared package, it is that application with an extra directory in the path.
* **`apps/*` depending on `apps/*`.** The two clients have nothing in common that is not a
  package; the webapp is a task manager and the website is a brochure.
* **Any Go code under `apps/` or `packages/`.** Nothing there is importable from Go, and no `.go`
  file is committed under either. The traffic runs the other way: the design system *generates*
  one Go file into the core (§6).
* **A JavaScript toolchain in the path of a Go build.** `go build ./...`, `go test ./...`,
  `golangci-lint run` and `make generate` work in a checkout where Node.js was never installed. A
  change that breaks that is a change that has to be reverted, not documented — which is why
  `presentation/webui/dist/index.html` is committed (ADR-0028) and why `make tokens` is a separate
  target from `make generate` (ADR-0029).

Tooling: `build/lint-workspace-map.mjs` (root script `pnpm lint:workspace`), the pnpm counterpart
of `gate-architecture`. It reads the manifests and the imports — a manifest cannot see a deep or
relative import that crosses a member boundary — and fails on any edge outside the map above. CI
runs it in every Node lane, selftest first: `--selftest` proves each forbidden edge kind is still
caught before the real tree is trusted.

---

## 3. Conventions

| Topic | Rule |
|---|---|
| File names | PascalCase as in the template (`WorkItem.go`, `CreateWorkItem.go`); ports are always called `Port.go` |
| Package names | Short, lower case, no underscores (`work`, `scheduling`) |
| One use case | One file, one `Command`/`Query` struct, one `Handler` with `Execute(ctx, cmd) (result, error)` |
| Constructors | `NewWorkItem(...) (WorkItem, error)` — invariants are checked in the constructor, never afterwards |
| Errors | Typed sentinel/struct errors in `domain/model/shared/Errors.go`, usable with `errors.Is/As` |
| Context | `context.Context` is always the first argument; actor and tenant travel via `ActorContext` (a typed wrapper, no bare context value access in business code) |
| Time/randomness/IDs | Exclusively through ports |
| Logging | `log/slog`, structured, no PII in logs, always with `request_id`/`trace_id` |
| Transactions | Exclusively in the application layer through `UnitOfWork`; repositories never open transactions |
| DTOs | The application layer owns its own input/output types; domain objects do not leave the core |
| Generated code | `presentation/openapi`, `infrastructure/postgres/sqlc`, `api/mcp` — never change by hand, generation lives in `make generate`. Two more live outside it: `core/domain/model/shared/LabelTokens.go` (`make tokens`) and `packages/api-client/dist` (`make api-client`), because both need Node.js and `make generate` must not (§6) |
| Tests | Domain = table tests without mocks; application = fakes of the ports; infrastructure = Testcontainers |
| Comments | Public types and functions carry a godoc comment; business rationale goes into an ADR, not into the code |

---

## 4. Split into deployment units

One module, one image, several roles. The automation context is cut so that it runs as its own
process without a code change:

| Role | Starts | Communication |
|---|---|---|
| `api` | REST, MCP, SSE, ICS, intake, admin | Reads/writes the database |
| `worker` | Outbox dispatcher, webhook delivery, mail, media GC, indexing | Database queue |
| `scheduler` | Reminders, occurrences, retention | Database queue + advisory lock |
| `automation` | The rule engine | Consumes events, calls use cases directly (in-process) or over HTTP when deployed separately |

The loops that make `worker` and `scheduler` real live in `presentation/worker/`, beside `rest` and
`mcp` rather than in `infrastructure/`. A job arriving and a handler running differs from a request
arriving and a handler running in who asked, not in what the layer does with it — so the queue is
an inbound adapter, and like every inbound adapter it reaches the outbound side through ports only.

A later genuine service split means only this: its own `cmd/automation/main.go`, and swapping the
in-process use case call for an HTTP/Connect client behind the same interface.

---

## 5. Deviations from the template

The template is a starting point, not a constraint. What was changed when this project was set up:

1. `go.mod` set to `github.com/Jersyfi/hubtask`.
2. The template's example files removed (`core/domain/model/example/Example.go`,
   `core/domain/service/DomainService.go`, `core/application/service/ApplicationService.go`,
   the placeholder `core/application/repository/Port.go` and `core/port/api/Port.go`).
   `core/shared/HelperFunctions.go` is being replaced by real utilities as they appear, and
   `presentation/rest/RestController.go` will be replaced by the generated router.
3. `core/port/environment/Port.go` extended with the `HUBTASK_*` variables; it grows with the
   configuration surface.
4. **CI/CD runs on GitHub Actions, not GitLab** ([ADR-0022](../adr/ADR-0022-github-platform.md)).
   The template's `example-gitlab-ci.yml` does not apply. Every gate is a `make` target, and the
   workflows under `.github/workflows/` only orchestrate — see
   [ci-cd.md](./ci-cd.md).
5. `k8s/Chart.yaml` carries the real project name; `values.yaml` gains roles, probes, HPA, and the
   migration hook as those are implemented.
6. Repository configuration (branch protection, environments, labels, required checks) is
   maintained through the GitHub UI and documented in [ci-cd.md](./ci-cd.md) §4. There is no
   bootstrap script — a one-shot setup script would rot, and the settings are declared where they
   are enforced.

---

## 6. Generated files that are committed

Generated output is not committed. Exactly two files break that rule, both deliberately, and each
for a reason that is about somebody who has not installed Node.js:

| File | Produced by | Why it is committed |
|---|---|---|
| `presentation/webui/dist/index.html` | a placeholder, replaced by the container build | `//go:embed all:dist` refuses to compile against a directory that does not exist, so without it `go build ./...` would need a frontend build ([ADR-0028](../adr/ADR-0028-embedded-web-ui.md)) |
| `core/domain/model/shared/LabelTokens.go` | `make tokens`, from `packages/design-system/tokens/tokens.json` | the domain validates a `colorToken` against it, and committing it keeps `go build ./...` working without Node — and turns a drift between the design system and the domain into a diff ([ADR-0029](../adr/ADR-0029-design-system-tokens.md)) |

**Neither may be edited by hand.** `LabelTokens.go` carries the `// Code generated … DO NOT EDIT.`
line, and CI regenerates it and fails on any difference. It holds the *names* of the ten label
colours and never a colour value: the core stays colour-blind while sharing one vocabulary with
the frontend, which is what `domain-model.md` §4 asks for when it stores a token instead of a hex.

Everything else the workspace produces — `packages/design-system/dist/`,
`packages/api-client/dist/`, `apps/*/dist/` — is ignored, and reproducible from the source plus
the pnpm lockfile.
