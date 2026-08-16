# ADR-0002 — A modular monolith with a separately deployable automation service

**Status:** accepted · **Date:** 2026-08-14

## Context
The application must be operable by private individuals on a small server and at the same time
scale horizontally for service providers. Around 14 bounded contexts have been identified.
Automation is the most load-intensive and riskiest part (third-party HTTP targets, rule storms,
long runtimes).

## Decision
One Go module, one repository, **one** deployment artefact with clear context boundaries enforced in
CI. The roles `api`, `worker`, `scheduler`, and `automation` are activatable process profiles of the
same binary. The automation context is cut so that it can run as its own process/deployment without
a code change; communication happens through events (the outbox) rather than direct coupling.

## Options
1. **A modular monolith (chosen).**
2. Microservices per context — distributed transactions, 14 deployments, self-hosting practically impossible.
3. A monolith without context boundaries — quick in the short term, but it prevents any later split.

## Consequences
**Positive:** simple operation for private users; ACID transactions within a use case; refactoring
across context boundaries stays possible; scalable through roles and replicas.
**Negative:** a shared failure domain for `api`; the risk that boundaries erode; one deployment
artefact for everything means a shared release cadence.
**Countermeasures:** import linting between contexts, communication between contexts preferably
through events, a separate `automation` deployment as the first split, load tests.
