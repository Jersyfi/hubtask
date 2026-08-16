# ADR-0004 — API first with OpenAPI 3.1 and code generation

**Status:** accepted · **Date:** 2026-08-14

## Context
Frontend design and feature set are deliberately left open. At the same time, n8n/Zapier, AI agents,
calendar clients, and later several of our own frontends should all use the same features. The API
is therefore the actual product, and it must not come into being as a by-product of the code.

## Decision
`api/openapi.yaml` (OpenAPI 3.1) is the **single source of truth** and is written before the
implementation. Server interfaces and client SDKs are generated with `oapi-codegen`; a CI job fails
on any divergence between the specification and the code. In addition: one major path `/api/v1`,
errors per RFC 9457 with stable codes, cursor pagination, `Idempotency-Key`, `ETag`/`If-Match`,
action suffixes (`:complete`), a query DSL for every view, and a capability manifest under
`/meta/capabilities`. An OpenAPI diff against the last tag detects breaking changes automatically.

## Options
1. **Spec-first REST plus code generation (chosen).**
2. Code-first with a generated specification — the specification follows the code, and breaks surface too late.
3. GraphQL — flexible queries, but difficult compatibility guarantees, expensive caching, and a poorer fit for automation platforms and webhooks.
4. gRPC as the public API — impractical for browsers and for Zapier/n8n; it remains an option for internal service-to-service communication.

## Consequences
**Positive:** a stable, documented, testable interface; clients (including an n8n node, a Zapier app,
and MCP tools) are generatable; compatibility is machine-checkable; a frontend can appear later
without a backend change.
**Negative:** maintaining the specification is extra work; a large YAML file; generated code must be
kept in the repository or reproducibly regenerated.
**Countermeasures:** split the specification across files with `$ref`, make `make generate` a
mandatory step, and run contract tests against the specification.
