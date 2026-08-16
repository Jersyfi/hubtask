# ADR-0012 — AI first through an MCP server and an AI port

**Status:** accepted · **Date:** 2026-08-14

## Context
The core is to be built "AI first" and with an eye to the future. At the same time the application
must not depend on any AI provider: self-hosters do not want a cloud dependency, companies have data
protection requirements, and models and protocols change quickly.

## Decision
Two separate directions:
1. **Inbound (agents operate Hubtask):** an MCP server as a presentation adapter under `/mcp`.
   Tools are generated from the use case registry, carry read-only/destructive hints, and run
   through service accounts with scopes; every action is audited with `actor.type = AI_AGENT`.
   Destructive operations are blocked for agents by default.
2. **Outbound (Hubtask uses AI):** `core/port/ai/Port.go` with `Complete`/`Embed`; adapters for
   OpenAI-compatible APIs, local Ollama, and `NoopAi` (the default). AI results are always
   suggestions with provenance, run asynchronously as jobs, are opt-in per tenant, and influence no
   invariants. Content from items and comments counts as data, not as instructions.

## Options
1. **An MCP adapter plus an AI port (chosen).**
2. AI features directly in the domain — untestable, not switchable off, and a third-party dependency in the core.
3. REST only for agents, no MCP — it works, but it gives up the easy connection to agent clients; MCP is cheap as an adapter.
4. Waiting until the protocols consolidate — gives up the head start; since MCP is only an adapter, the risk is small.

## Consequences
**Positive:** the application is fully usable without AI (QS-09); switching provider is switching an
adapter; a new agent protocol is a new adapter, not a rebuild; the agent-friendly properties
(idempotency, ETag, machine-readable errors, the capability manifest, dry run) also benefit human
clients.
**Negative:** two inbound channels (REST, MCP) must stay in sync; MCP is a young protocol with a
risk of change; agent access widens the attack surface.
**Countermeasures:** tool generation from one registry (no hand-maintained duplicate); a parity test
in CI; narrow scopes, rate limits, and a confirmation requirement for destructive tools; prompts as
versioned resources.
