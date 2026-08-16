# Working instructions for Claude Code

This file is read automatically at the start of every session. It is binding.

## What this project is

Hubtask is a task management system with five levels (Hub → Collection → Task → Work Package →
Activity), multi-tenant, offline-capable, Go + PostgreSQL, hexagonal architecture.
**The architecture is fully decided and documented.** It gets implemented, not redesigned.

## Reading order

1. `docs/architecture/arc42.md` — chapters 1, 4, 5, 8
2. `docs/architecture/domain-model.md` — aggregates, capability matrix, use case catalogue
3. `docs/architecture/project-structure.md` — where each kind of code belongs
4. `docs/architecture/api-guidelines.md` — before you touch any endpoint
5. The subject document matching your task (`security`, `audit`, `data-protection`,
   `data-retention`, `backup-restore`, `offline-sync`, `observability-reliability`, `automation`,
   `i18n-l10n`, `multi-tenancy`, `ai-first`)
6. The ADRs named in the task

Read selectively, not exhaustively. But read **completely** what you do read — half-knowledge of
the capability matrix produces false invariants.

## Rules that do not bend

These rules take precedence over any task. If a task contradicts them, the task is wrong — report
that instead of breaking the rule.

| # | Rule | Origin |
|---|---|---|
| 1 | `core/domain` and `core/port` import **no** third-party libraries and nothing from `infrastructure/` or `presentation/`. Dependencies always point inwards. | ADR-0001 |
| 2 | Authorisation happens **exclusively** in the application layer, never in an adapter, never in a repository. | ADR-0005 |
| 3 | Every database query goes through the transaction wrapper that sets `SET LOCAL app.tenant_id`. Never use `pgxpool` directly. | ADR-0010 |
| 4 | No `time.Now()`, no `math/rand`, no UUID generation in `core/domain` or `core/application` — only through the `Clock`, `RandomSource`, and `IDGenerator` ports. | arc42 §8.13 |
| 5 | No bare goroutines. Concurrency only through `core/shared/concurrency.SafeGo`. | ADR-0016 |
| 6 | Every outbound HTTP call goes through `infrastructure/httpclient.GuardedClient`. Never `http.DefaultClient`. | ADR-0015, T-07 |
| 7 | No call without a timeout or a context deadline. | ADR-0016 |
| 8 | No display text in the backend. Message codes plus parameters only. | ADR-0011 |
| 9 | SQL only parameterised, through sqlc. Never string concatenation to build a query, not even for filters. | ADR-0015, T-06 |
| 10 | No user content (titles, notes, comments) in logs, metrics, traces, or audit entries. | ADR-0017, ADR-0018 |
| 11 | `openapi.yaml` is the source, not the result. Change the specification first, then `make generate`, then implement. Never hand-edit generated code. | ADR-0004 |
| 12 | Migrations are forward-only and safe for rolling updates (expand/contract). Never change an existing migration. | ADR-0003 |
| 13 | English everywhere: documentation, code, identifiers, code comments, commit titles, and commit bodies. | Project convention |

## The loop for every task

1. **Understand**: read the task, read the documents it names, locate the use case in the
   catalogue in `domain-model.md`. If something contradicts the documentation, ask — do not guess.
2. **Plan**: a short plan naming the affected files, before you write anything. If more than
   about 8 files are involved, split the task into steps and finish them one at a time.
3. **Specification first**: for API changes `api/openapi.yaml`, for data model changes a migration
   in `db/migrations/` and queries in `db/queries/`, then `make generate`.
4. **Implement from the inside out**: domain → application → ports → adapters → presentation.
5. **Test**: domain logic with table tests and no infrastructure. Repositories with Testcontainers.
   A cross-tenant negative test for every new repository method — otherwise gate SG-3 fails.
6. **Check**: `make verify` must be green locally before you open a PR.
7. **Finish**: Conventional Commit, fill in the PR template completely, work through the
   Definition of Done in `docs/architecture/engineering-guidelines.md` §3.

## Definition of Done (short form — the long form governs)

A piece of work is finished when, in addition to working code:

- The use case is registered in the registry → available via REST, MCP, and automation (parity test)
- A metric and a trace span exist (gate RT-12)
- Any auditable action is in the `AuditableAction` registry (gate AU-1)
- New personal data fields are in the data catalogue with a deletion path
- A merge rule is defined for every new field for offline synchronisation (LWW, OR-set,
  fractional index, or server-side) — see `offline-sync.md` §4
- Message codes are in `locales/en.json`
- `make verify` is green, and `make generate` produces no diff

## Commands

```bash
make tools              # install the tools once
make verify             # all fast gates — the local equivalent of the PR check
make gate-unit          # tests only
make gate-architecture  # layer boundaries, goroutine ban, parity checks
make gate-security      # SG-1..SG-12
make gate-selftest      # proves the gates catch a deliberate violation of every rule
make generate           # generate code from openapi.yaml and db/queries
make db-up              # start a local PostgreSQL
make migrate            # apply the migrations
make run ROLES=api      # start the server locally
```

## What you do not decide yourself

Report back instead of acting on your own for:

- Any deviation from an ADR or a subject document
- Any new third-party dependency (every dependency is a supply chain decision)
- Any change to `api/openapi.yaml` that renames or removes an existing field
- Any change to the licence model, the security gates, or the retention safeguards
- Anything that could irrecoverably delete user data

In those cases, write a draft ADR under `docs/adr/` rather than a pull request presenting a fait
accompli.

## Style

- Small, self-contained changes. One PR = one use case or one clearly bounded building block.
- Errors are typed values, not strings; wrap with `%w`.
- Comments explain the *why*, not the *what*. Reference ADR numbers where a decision sits behind
  the code.
- No speculative abstractions for a hypothetical future. The generalisation is already in the
  domain model; anything beyond that waits for a real second use case.
