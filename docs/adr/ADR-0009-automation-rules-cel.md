# ADR-0009 — Automation rules with CEL instead of scripting

**Status:** accepted · **Date:** 2026-08-14

## Context
Users should be able to "build any conceivable feature themselves" — through external tools (n8n,
Zapier) and through internal automation. The internal variant runs on the operator's server
resources and is therefore an attack and load vector (infinite loops, resource consumption, data
exfiltration, SSRF).

## Decision
A declarative rule model (trigger → conditions → actions). Conditions and text templates are
expressed in **CEL (Common Expression Language, `cel-go`)**: no loops, no I/O, terminating, with a
cost and time limit. Actions are exclusively registered use cases plus `SEND_WEBHOOK`/`HTTP_REQUEST`
through a hardened HTTP client. Rules run under a `run_as` account and can never hold more rights
than it does. Protective mechanisms: a causality depth (5 by default), throttling and dedupe per
rule, quotas per tenant, automatic deactivation after repeated errors, a dry-run endpoint, and a
complete `RuleRun` log.

## Options
1. **Declarative rules + CEL (chosen).**
2. Embedded scripting (Lua, Starlark, JavaScript via goja) — maximally flexible, but sandboxing, resource, and auditing problems; risky in multi-tenant operation.
3. WASM plugins — strong isolation, but high complexity and poor accessibility for end users; it remains an option for later.
4. External automation only (n8n/Zapier) — shifts the effort onto users and does not meet the "internal automation service" requirement.
5. Our own expression language — effort and risk of defects with no advantage over CEL.

## Consequences
**Positive:** predictable runtime and cost; safe multi-tenancy; rules are data (exportable,
versionable, creatable through the API or MCP); the action list grows automatically with the use
case catalogue; a UI rule builder becomes possible later without a backend change.
**Negative:** less powerful than real scripting (no arbitrary control flow, no external data sources
in conditions); CEL needs explaining to end users.
**Countermeasures:** the `BRANCH`/`WAIT` actions cover the common flows; an expression library with
examples; rule templates; for special cases the complete external API is available; WASM stays
documented as an extension path.
