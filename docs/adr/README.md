# Architecture Decision Records (ADR)

Format: lightweight MADR. One file per decision, immutable once accepted — a change produces a new
ADR that supersedes the old one (`Supersedes ADR-xxxx` / `Superseded by ADR-yyyy`).

Status: `proposed` · `accepted` · `rejected` · `deprecated` · `superseded`

| ADR | Title | Status | Concerns |
|---|---|---|---|
| [0001](./ADR-0001-hexagonal-architecture.md) | Hexagonal architecture following the in-house template | accepted | Structure |
| [0002](./ADR-0002-modular-monolith.md) | A modular monolith with a separately deployable automation service | accepted | System boundaries |
| [0003](./ADR-0003-postgresql-as-single-datastore.md) | PostgreSQL as the only mandatory datastore | accepted | Persistence, operations |
| [0004](./ADR-0004-api-first-openapi.md) | API first with OpenAPI 3.1 and code generation | accepted | Interfaces |
| [0005](./ADR-0005-authn-authz.md) | OIDC, local accounts, tokens, and RBAC | accepted | Security |
| [0006](./ADR-0006-generalized-workitem.md) | A generalised WorkItem with capability profiles | accepted | Domain model |
| [0007](./ADR-0007-events-outbox-cloudevents.md) | Transactional outbox and CloudEvents | accepted | Integration |
| [0008](./ADR-0008-jobs-and-scheduling.md) | Jobs, scheduling, and leader election in PostgreSQL | accepted | Operations |
| [0009](./ADR-0009-automation-rules-cel.md) | Automation rules with CEL instead of scripting | accepted | Automation |
| [0010](./ADR-0010-multi-tenancy.md) | Multi-tenancy through a shared schema and RLS | accepted | Security, scaling |
| [0011](./ADR-0011-i18n-message-codes.md) | i18n through message codes rather than server text | accepted | i18n |
| [0012](./ADR-0012-ai-first-mcp.md) | AI first through an MCP server and an AI port | accepted | AI |
| [0013](./ADR-0013-licensing.md) | The BSL 1.1 licence model with a conversion to Apache-2.0 | accepted | Legal, product |
| [0014](./ADR-0014-single-image-multi-role.md) | One container image, several roles | accepted | Deployment |
| [0015](./ADR-0015-security-baseline.md) | Security as an enforced baseline with CI gates | accepted | Security, process |
| [0016](./ADR-0016-observability-reliability.md) | Self-diagnosis, controlled degradation, SLOs | accepted | Operations, reliability |
| [0017](./ADR-0017-audit-trail.md) | The audit trail: append-only, chained, content-free | accepted | Audit, compliance |
| [0018](./ADR-0018-privacy-by-design.md) | Data protection by design and by default | accepted | Data protection, domain |
| [0019](./ADR-0019-backup-targets.md) | Backup as an application feature with freely chosen targets | accepted | Operations, data |
| [0020](./ADR-0020-retention-policies.md) | Retention rules as data, with a grace period | accepted | Domain, data protection |
| [0021](./ADR-0021-offline-sync.md) | Offline sync: server-authoritative, per-field merging | accepted | API, domain, clients |
| [0022](./ADR-0022-github-platform.md) | GitHub as the platform, Actions as the pipeline, GHCR as the registry | accepted | Operations, process, supply chain |
| [0023](./ADR-0023-deployment-strategy.md) | Push-based deployment with approval, GitOps-ready | accepted | Operations, delivery |
| [0024](./ADR-0024-tenant-scoped-foreign-keys.md) | Tenant-scoped foreign keys | accepted | Security, persistence |
| [0025](./ADR-0025-precondition-failures.md) | The status of a failed precondition | accepted | Interfaces, API |
| [0026](./ADR-0026-query-dsl-sql-construction.md) | How the query DSL turns into SQL | accepted | Security, persistence, API |
| [0027](./ADR-0027-monorepo-structure.md) | One repository for the core, the clients, and the design system | accepted | Structure, process |
| [0028](./ADR-0028-embedded-web-ui.md) | The web UI ships inside the binary, as an adapter | accepted | Deployment, security, clients |
| [0029](./ADR-0029-design-system-tokens.md) | The design system is code, and `tokens.json` is its only origin | accepted | Design system, domain |
