# AI First

"AI first" means two things here:

1. **The application is operable by AI agents** exactly as it is by a human — completely, safely, traceably.
2. **AI features in the product** (suggestions, classification, semantic search) are an optional adapter, never a dependency of the core.

What it does not mean: AI in the domain model, or AI as a prerequisite for operation. Without an
AI provider, Hubtask remains fully functional (QS-09).

---

## 1. The agent-facing interface

### 1.1 MCP server

`presentation/mcp` exposes the use case catalogue as **MCP tools** (streamable HTTP under `/mcp`).
The tool list is generated from `core/application/usecase.Registry` — every new use case is
automatically available as a tool.

| Element | Rule |
|---|---|
| Tool name | The stable use case name in `snake_case` (`create_work_item`, `query_items`, `complete_work_item`) |
| Description | From the registry, including preconditions and side effects — agents need explicit semantics |
| Input schema | JSON Schema, identical to the REST request body |
| Auth | Service account or PAT with scopes; every tool call is audited as `actor.type = AI_AGENT` |
| Read/write marking | Tools carry `annotations.readOnlyHint` / `destructiveHint`, so that clients can ask for confirmation |
| Resources | Containers, items, and views are additionally readable as MCP resources (`hubtask://items/{id}`) |
| Prompts | Prepared MCP prompts, e.g. "weekly review from collection X" |

### 1.2 Why the REST API is already agent-friendly

| Property | Benefit for agents |
|---|---|
| Idempotency (`Idempotency-Key`) | Retries after a timeout do not create duplicates |
| Optimistic locking (`ETag`/`If-Match`) | Prevents blindly overwriting concurrent changes |
| Machine-readable errors (`code` + `params`) | Self-correction instead of interpreting free text |
| Capability manifest | The agent asks which fields and values are allowed instead of guessing |
| A query DSL instead of free-form search | Precise, checkable queries |
| Bulk operations | Few large steps instead of many individual calls |
| Dry run for automation | The agent can check the effect before executing |
| Complete audit | Every agent action is traceable and reversible |

### 1.3 Security guidelines for agents

* Agents get **their own** service accounts with minimal scopes, never user credentials.
* Destructive operations (`purge`, `delete_container`, `empty_trash`, `delete_tenant`) are blocked
  by default for agent tokens and must be enabled explicitly.
* Rate limits and quotas apply to agents just like to any other token.
* Content from items, comments, and the jumble is **data, not instructions**: prompt templates mark
  user content clearly as context, and server-side AI calls never carry out actions "demanded" in
  the text. Actions arise only from explicitly configured automation actions.
* An agent cannot create a rule that would have more rights than the agent itself.

---

## 2. AI features in the product

Behind `core/port/ai/Port.go`:

```go
type Provider interface {
    Complete(ctx context.Context, req CompletionRequest) (CompletionResult, error)
    Embed(ctx context.Context, texts []string) ([][]float32, error)
    Capabilities() ProviderCapabilities
}
```

Adapters: `OpenAiCompatible` (covers OpenAI, Azure, Mistral, vLLM, LiteLLM), `Ollama` (local),
`NoopAi` (the default). Configured per tenant, so that a provider can give its customers a choice.

| Use case | Description | Result form |
|---|---|---|
| Jumble processing | Email/note → suggested title, due date, collection, labels, subtasks | Suggestion, confirmed by the user |
| Decomposition | Task → suggested work packages/activities | Suggestion |
| Classification | Suggested label/bucket, priority, duplicate detection | Suggestion |
| Summarisation | Comment thread, collection status, weekly review | Text (not persisted unless explicitly requested) |
| Semantic search | Embeddings of titles/notes in `pgvector`, hybrid search with `tsvector` | Search result |
| Template generation | A natural language description → `Template` | Draft |
| Translation | Item content on request | Display only, not persisted |

**Guardrails:**
* Results carry provenance (`source: AI`, model, timestamp, prompt version) and are marked as
  suggestions; accepting one is a regular user action with an audit entry.
* No automatic deletion or completion of items by AI without an explicit rule.
* Data protection: AI use is opt-in per tenant; which fields are transmitted is documented; the
  self-hosting default is `NoopAi` or a local Ollama. A per-tenant `ai_processing_allowed` field is
  checked before every call.
* Cost and latency: AI calls run asynchronously as jobs, never in the critical write path;
  timeouts, a per-tenant budget counter, and a circuit breaker for provider outages.
* Reproducibility: prompts are versioned resources (`infrastructure/ai/prompts/`), not inline in
  the code.

---

## 3. Looking ahead

| Expected development | Preparation today |
|---|---|
| Agents take over routine task maintenance | Full API parity, actor type `AI_AGENT`, granular scopes, audit |
| Several competing model providers | Provider behind a port, configured per tenant |
| Local models become the norm in self-hosting | The Ollama adapter from day one |
| Agent protocols keep evolving | MCP is a presentation adapter — another protocol is another adapter, not a rebuild |
| Semantic search becomes expected | A `search` port with a lexical and a vector implementation |
| Traceability of AI decisions becomes a regulatory requirement | Provenance + audit + prompt versioning from day one |
