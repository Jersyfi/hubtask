# ADR-0001 — Hexagonal architecture following the in-house template

**Status:** accepted · **Date:** 2026-08-14

## Context
A binding in-house template exists (*Go hexagonal template*) with the structure `core/domain`,
`core/application`, `core/port`, `presentation/rest`, and the convention that interfaces live in
files named `Port.go`. In addition, "Explicit Architecture" (Herberto Graça) is prescribed as the
conceptual reference: DDD, hexagonal, onion, clean, CQRS. The application is meant to live a long
time, to acquire many adapters (REST, MCP, calendar, mail, webhooks), and to serve several
operating models.

## Decision
The template structure is adopted and extended with the directories `infrastructure/`, further
`presentation/` adapters, `cmd/`, `api/`, `db/`, `locales/`, and `test/`
(details: [project-structure.md](../architecture/project-structure.md)).
The core (`core/**`) stays free of technology: no HTTP, no SQL, no JSON tags, no framework types.
All external systems are attached through ports that are defined in the core and implemented in
`infrastructure/**`. The direction of dependencies is enforced in CI by architecture tests.

Light CQRS: commands go through aggregates and invariants, while queries may use optimised read
models or direct projections (the query DSL) — without a separate event store.

## Options
1. **Adopt the template (chosen)** — the in-house standard, recognisability, testability.
2. The standard Go layout (`internal/`, `pkg/`) — violates the requirement and brings no advantage.
3. A layered architecture without ports — faster initially, but technology seeps into the domain; untenable given the planned number of adapters.

## Consequences
**Positive:** the domain is testable without infrastructure (millisecond tests); new adapters (MCP,
CalDAV, NATS) are additive; replacing a technology stays locally contained; a familiar structure for
the team.
**Negative:** more files and mapping code (domain ↔ DTO ↔ row); discipline is needed so that
application services do not become anaemic pass-throughs; newcomers need time to get oriented.
**Countermeasures:** code generation for the mapping-adjacent parts (sqlc, oapi-codegen),
architecture tests, and an example use case as a reference implementation.
