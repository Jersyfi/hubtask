# ADR-0098 — The suite and its foundation

**Status:** accepted · **Date:** 2026-10-28 · **Decided:** 2026-10-28, by the owner (the concept
worked out with him and accepted as a whole; [the record](../backlog/suite/accepted.md))

**Rule lives in:** [principles.md](../vision/principles.md) (P-17, P-18, P-19),
[vision.md](../vision/vision.md), [non-goals.md](../vision/non-goals.md); until each foundation
change lands with its step in its subject document (`domain-model.md`, `multi-tenancy.md`,
`offline-sync.md`, …), its target is in [backlog/suite/foundation.md](../backlog/suite/foundation.md)
and the order in [backlog/suite/README.md](../backlog/suite/README.md)

## Context

Hubtask was "a task manager that is genuinely yours". Companies that must stay on their own
premises — public sector, health, finance, defence — lose their issue tracker's on-premises edition
in 2029 and ask in the first hour for configurable workflows, dependencies and a Gantt chart, a
wiki, a service desk and an asset database; the research is in
[backlog/suite/research/](../backlog/suite/research/products-2026-10.md). An audit of `main` at
b832f6ceb found that the core's guarantees (tenant boundary, trail, registry as the only door,
sync, gates) are generic, while types, fields, relations, completion, settings, doors, roles, sync
mutations, the query DSL and scale are not ([foundation.md](../backlog/suite/foundation.md) §1,
A-1 … A-10). Building the suite's products on that shape would grow a second solution per feature.

## Decision

1. **Hubtask becomes a work suite** — workflows and types, planning, knowledge, whiteboards,
   service desk with intake and approvals, assets and CMDB, time, goals and insight, governed
   agents, integrations ([products.md](../backlog/suite/products.md), M1 … M10) — under three new
   principles: **P-17** depth is switched on and starts from a default that works; **P-18** one
   feature, one building block, every door; **P-19** everything is an object you can point at.
   Six non-goals bound it: chat, meetings, invoicing, in-process plugins, a spreadsheet engine, a
   sales CRM.
2. **The foundation changes first**, in thirteen steps F-1 … F-13: a type registry replacing the
   `item_type` enum (records without completion included), typed fields, links and references,
   forms and workflows inside every writing use case, one settings catalogue, the configuration
   document with plan and apply, registry-driven doors and custom roles, generic sync with a
   realtime document service, query v2 and a search port, scale and fairness, a connector framework
   with reconciliation, governed agents, compliance by design.
3. **The new third-party dependencies are accepted with the concept**
   ([foundation.md](../backlog/suite/foundation.md) §3): a Yjs-compatible CRDT and its update
   format, a ProseMirror-based block editor (MIT parts only), a canvas engine for whiteboards, a CEL
   evaluator for the browser, a websocket library for Go. Each passes the licence gate when it
   lands, or is replaced, never excepted; none enters `core/domain` or `core/port` (rule 1).
4. **It is built in concept mode** ([ADR-0090](./ADR-0090-concept-mode-findings-fixed-where-found-proof-by-tests.md)),
   split by area into the steps S-00 … S-23 on one integration branch, **one coding worker at a
   time**, steered by an orchestrating session that keeps the plan's state in the repository; the
   branch enters `main` through one pull request after a demo of the running app (the owner's word
   of 2026-10-28, overriding concept mode's default split into several pull requests). Nothing enters the `0.9.5`
   scope window; the suite starts with `1.1.0`.

## Alternatives

* **Stay a task manager** and polish the personal and team deployments — safer, a small market, no
  answer to companies leaving their on-premises tracker.
* **Integrate other open-source tools** (a wiki, a ticket system) instead of building — breaks one
  trail, one backup, one API and one tenant boundary, which is the product's reason to exist.
* **Build the products on today's core** — every product would add its own types, fields, settings
  and doors; the split P-18 forbids would be built in.

## Consequences

`domain-model.md` §2's claim that a new type needs no schema change was false and is corrected; it
becomes true with F-1. `UC-WRK-03` no longer rules out dependencies. The questions that stay the
owner's when they arise — a lifecycle that would irrecoverably delete records, the retention of
agent run traces, a change to the security gates, a dependency that fails the licence gate, each
merge — are named in the [acceptance record](../backlog/suite/accepted.md).
