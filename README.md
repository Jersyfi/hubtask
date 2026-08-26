# Hubtask

Task management with five levels — **Hub → Collection → Task → Work Package → Activity** —
for private individuals and for service providers. Backend first, API first, AI first.
Go, PostgreSQL, hexagonal architecture.

* **Self-hosting** with Docker/Podman: two containers, the full feature set, no limitations.
* **Platform operation** with Kubernetes: the same image, multi-tenant, horizontally scalable.
* **Automatable** through a REST API, webhooks, n8n/Zapier, and an internal rule engine.
* **Multilingual** without backend changes; any language, time zone, and text direction.
* **Agent-ready** through an MCP server; AI features are optional and can be switched off.
* **Secure by default** — the tenant boundary is enforced in the database, twelve security gates run in the pipeline.
* **Self-diagnosing** — degrades in a controlled way instead of crashing, and reports what it is missing through `/meta/health`.
* **Auditable and GDPR-ready** — a chained, content-free audit log; data subject rights as use cases with deadline tracking.
* **Freely backed up** — choose the schedule and the target yourself (S3, SFTP, FTP, WebDAV, cloud, local), encrypted, with retention and restore down to item level.
* **Usable offline** — clients keep working without a network; concurrent changes by others are not lost when merging.

---

## Documentation

The architecture is fully documented and ready to implement. Start here:

| Document | Contents |
|---|---|
| [docs/architecture/arc42.md](./docs/architecture/arc42.md) | **Main document**, following arc42: goals, constraints, context, solution strategy, building blocks, runtime, deployment, cross-cutting concepts, quality, risks, glossary |
| [docs/architecture/domain-model.md](./docs/architecture/domain-model.md) | Aggregates, capability matrix, invariants, events, use case catalogue |
| [docs/architecture/project-structure.md](./docs/architecture/project-structure.md) | Go directory tree, dependency rules, conventions |
| [docs/architecture/api-guidelines.md](./docs/architecture/api-guidelines.md) | Resources, query DSL, errors, idempotency, versioning |
| [docs/architecture/multi-tenancy.md](./docs/architecture/multi-tenancy.md) | Tenant isolation, RLS, quotas, GDPR |
| [docs/architecture/security.md](./docs/architecture/security.md) | Threat model (STRIDE), hardening, crypto, supply chain, security gates |
| [docs/architecture/audit.md](./docs/architecture/audit.md) | Tamper-evident audit trail, catalogue, verification, SIEM export |
| [docs/architecture/data-protection.md](./docs/architecture/data-protection.md) | GDPR: roles, data subject rights, deletion concept, data residency |
| [docs/architecture/backup-restore.md](./docs/architecture/backup-restore.md) | Backup targets, schedules, encryption, restore, import |
| [docs/architecture/data-retention.md](./docs/architecture/data-retention.md) | Configurable retention periods for business data |
| [docs/architecture/offline-sync.md](./docs/architecture/offline-sync.md) | Offline operation, conflict resolution, client requirements |
| [docs/privacy/data-catalog.md](./docs/privacy/data-catalog.md) | Record of processing activities with deletion paths per storage location |
| [docs/architecture/observability-reliability.md](./docs/architecture/observability-reliability.md) | SLOs, metric and alert catalogue, resilience patterns, self-diagnosis |
| [docs/architecture/i18n-l10n.md](./docs/architecture/i18n-l10n.md) | Languages, time zones, search, RTL |
| [docs/architecture/automation.md](./docs/architecture/automation.md) | Rule engine, triggers/actions, webhooks, n8n/Zapier |
| [docs/architecture/ai-first.md](./docs/architecture/ai-first.md) | MCP, agent security, AI features |
| [docs/architecture/versioning-release.md](./docs/architecture/versioning-release.md) | Semantic versioning, releases, migrations |
| [docs/architecture/licensing-editions.md](./docs/architecture/licensing-editions.md) | Licence and edition model |
| [docs/architecture/engineering-guidelines.md](./docs/architecture/engineering-guidelines.md) | Test strategy, Definition of Ready/Done, operations |
| [docs/architecture/deployment.md](./docs/architecture/deployment.md) | Environments, rollout, the integration environment, self-hosting |
| [docs/architecture/ci-cd.md](./docs/architecture/ci-cd.md) | GitHub Actions pipeline, quality gates, release automation |
| [docs/architecture/support-matrix.md](./docs/architecture/support-matrix.md) | What is supported, with the CI job that proves every row |
| [docs/design/design-system.md](./docs/design/design-system.md) | Design system specification: tokens, themes, foundations |
| [docs/adr/README.md](./docs/adr/README.md) | The architecture decision records with their reasoning |
| [docs/roadmap.md](./docs/roadmap.md) | Milestones `0.1.0` through `1.0.0` |
| [api/openapi.yaml](./api/openapi.yaml) | API contract (skeleton) |
| [db/schema.sql](./db/schema.sql) | Reference database schema |

---

## The architecture in one paragraph

One Go module, one container image, several process roles (`api`, `worker`, `scheduler`,
`automation`). The core (`core/`) is technology-free and knows only ports; REST, MCP, calendar,
mail, and webhooks are adapters. PostgreSQL is the only mandatory dependency and handles storage,
the job queue, the event outbox, full-text search, and tenant isolation (row level security).
Instead of four specialised level entities there is one generalised `WorkItem` aggregate root with
capability profiles — new levels and fields are configuration, not a migration.

The same repository also holds both first-party clients and the design system
([ADR-0027](./docs/adr/ADR-0027-monorepo-structure.md)): the web UI under `apps/webapp` is an
inbound adapter like REST or MCP and ships inside the binary
([ADR-0028](./docs/adr/ADR-0028-embedded-web-ui.md)), and the project website lives under
`apps/website`.

---

## Quick start

```bash
git clone https://github.com/Jersyfi/hubtask.git && cd hubtask
cp deploy/docker/.env.example .env      # set the secrets
docker compose -f deploy/docker/compose.yaml up -d
# API:  http://localhost:8080/api/v1
# Meta: http://localhost:8080/api/v1/meta/capabilities
```

Kubernetes:

```bash
helm upgrade --install hubtask ./k8s -f ./k8s/values.yaml
```

---

## The command line client

`hubctl` signs in with a personal access token and speaks the published contract — its types are
generated from `api/openapi.yaml`. Every command prints a table for a person, or, with `--json`,
the API's own payload for a pipe: exactly one document on standard output and every diagnostic on
standard error.

```bash
make build                                   # bin/hubctl
echo "$TOKEN" | bin/hubctl auth login --url http://localhost:8080
HUB=$(bin/hubctl container create --type HUB --name "Personal" | awk 'NR==2 {print $1}')
bin/hubctl container create --type COLLECTION --parent "$HUB" --name "Errands"
bin/hubctl item create --collection "$COLLECTION" --type TASK --title "Buy milk"
bin/hubctl item complete "$ITEM"
bin/hubctl comment add "$ITEM" --body "Done on the way home"
bin/hubctl search milk
bin/hubctl trash ls
bin/hubctl watch                             # follow the change stream; Ctrl-C ends it

bin/hubctl due set "$ITEM" --at 2026-09-10   # a day; a timestamp is a moment instead
bin/hubctl remind add "$ITEM" --at -PT30M    # half an hour before it is due
bin/hubctl recur set "$ITEM" --rule "FREQ=WEEKLY;BYDAY=MO" --zone Europe/Berlin
bin/hubctl template instantiate "$TEMPLATE" --collection "$COLLECTION" --anchor 2026-09-07
bin/hubctl view export "$VIEW" --format ICS --out week.ics
bin/hubctl calendar mint --view "$VIEW"      # prints the feed URL once; it is the credential
```

Errors are the message catalogue's sentences rather than the problem document behind them — the
server emits codes, never display text ([ADR-0011](docs/adr/ADR-0011-i18n-message-codes.md)).
Which platforms the binary is supported on is in
[support-matrix.md](docs/architecture/support-matrix.md) §4.

---

## Technology

| Area | Choice |
|---|---|
| Language | Go (≥ 1.26), `net/http`, `log/slog` |
| Database | PostgreSQL 16+ (`pgx/v5`, `sqlc`, `goose`) |
| API | OpenAPI 3.1 spec-first (`oapi-codegen`), RFC 9457, cursor pagination |
| Events | Transactional outbox, CloudEvents 1.0, optionally NATS JetStream |
| Automation | Declarative rules, conditions in CEL (`cel-go`) |
| Scheduling | RFC 5545 RRULE (`rrule-go`), PostgreSQL job queue |
| AI | MCP server; AI port with adapters for OpenAI-compatible APIs and Ollama |
| Object storage | S3/MinIO or a local volume |
| Observability | OpenTelemetry, Prometheus; dashboards, alert rules, and runbooks included |
| Resilience | Circuit breakers, bulkheads, load shedding, dead letter, idempotency throughout |
| Backup | Own logical archive format (JSON Lines + manifest), AES-256-GCM, adapters for S3/SFTP/FTPS/WebDAV/SMB/Azure/GCS/rclone/local |
| Offline sync | Delta sync via a change log, hybrid logical clocks, OR-sets, fractional indices |
| Clients | Svelte 5 + TypeScript; the webapp as a plain Vite SPA, served from inside the binary; Tauri 2 shells for desktop and mobile planned |
| Design system | Design tokens (W3C DTCG, Style Dictionary) as the single origin of every visual value; pnpm workspace |
| Deployment | Docker/Podman Compose, Helm for Kubernetes |

---

## Licence

Hubtask is published under the **Business Source License 1.1**. Each version converts to
**Apache-2.0** three years after it is first published.

**Free, with the complete feature set, for non-commercial use** — private individuals,
households, non-profits, teaching, research, and evaluation or development by anyone. Self-hosting
is unrestricted: there is no feature gate, no licence key, no telemetry.

**Commercial production use requires a commercial licence.** That includes companies running it
for their own operations, offering it as a service, and freelancers using it in their professional
practice. No-cost licences for non-profits, schools, and public bodies are granted on request.

Details: [LICENSE](./LICENSE) · [ADR-0013](./docs/adr/ADR-0013-licensing.md) ·
[licensing-editions.md](./docs/architecture/licensing-editions.md) ·
[TRADEMARK.md](./TRADEMARK.md). Licensing enquiries: licensing@hubtask.eu.

This is source-available software, not OSI open source, and the project does not claim otherwise.
Donations keep it free for private use — see [the funding section](./docs/architecture/licensing-editions.md#5-funding-and-what-happens-if-it-does-not-work)
for what happens if they stop covering maintenance.

---

## Contributing

Conventional Commits, trunk-based development, Definition of Ready/Done from
[engineering-guidelines.md](./docs/architecture/engineering-guidelines.md).
Architectural changes arrive as an ADR, not as a pull request without context.
See [CONTRIBUTING.md](./CONTRIBUTING.md), [CLA.md](./CLA.md), [SECURITY.md](./SECURITY.md),
and [CODE_OF_CONDUCT.md](./CODE_OF_CONDUCT.md).
