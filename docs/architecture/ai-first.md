# AI First

"AI first" means two things here:

1. **The application is operable by AI agents** exactly as it is by a human — completely, safely, traceably.
2. **AI features in the product** (suggestions, classification, semantic search) are an optional adapter, never a dependency of the core.

What it does not mean: AI in the domain model, or AI as a prerequisite for operation. Without an
AI provider, Hubtask remains fully functional (QS-09).

---

## 1. The agent-facing interface

### 1.1 MCP server

`presentation/mcp` exposes the use case catalogue as **MCP tools** (streamable HTTP under `/mcp`). Both halves of that transport are served since J-13: `POST /mcp` carries the client's calls, `GET /mcp` the server's notifications, and `Mcp-Session-Id` binds the two into one conversation.
The tool list is generated from `core/application/usecase.Registry` — every new use case is
automatically available as a tool.

| Element | Rule |
|---|---|
| Tool name | The stable use case name in `snake_case` (`create_work_item`, `query_items`, `complete_work_item`) |
| Description | From the registry, including preconditions and side effects — agents need explicit semantics |
| Input schema | JSON Schema, identical to the REST request body |
| Auth | Service account or PAT with scopes; every tool call is audited as `actor.type = AI_AGENT` |
| Read/write marking | Tools carry `annotations.readOnlyHint` / `destructiveHint`, so that clients can ask for confirmation |
| Resources | Containers, items and views are additionally readable as MCP resources (`hubtask://items/{id}`). **Shipped in J-11**: a resource is a read that is already in the catalogue, addressed by URI instead of by argument, so `resources/read` carries no authorisation of its own — exactly as `tools/call` carries none ([ADR-0051](../adr/ADR-0051-mcp-resources-are-catalogue-reads.md)). `resources/list` enumerates the hubs and the caller's saved views, paged; entries are reached by URI or found with `search_items`, because no unanchored item list exists and one built for this would be a tenant asking for everything |
| Session and stream | `initialize` hands out an `Mcp-Session-Id` and `GET /mcp` opens the server-initiated stream (J-13). The identifier is a **signed statement rather than a row**, so any pod checks it with no shared state — a map would bind a client to whichever pod answered its handshake. The stream is held to the *same* per-credential, per-tenant and per-process caps as `GET /stream`, in the same registry and the same metric: an agent's stream is not a different kind of connection from a browser's |
| Prompts | Prepared MCP prompts, e.g. "weekly review from collection X". **Shipped in J-12**: the same versioned files under `infrastructure/ai/prompts/` the outbound adapters read — one store, because two is how one prompt comes to exist in two versions. A prompt is published by describing itself; the four this product asks its own provider carry no title and stay unpublished. Arguments are declared and refused by name when unknown, and a resource argument becomes a link into the URI space above, so the permission is asked where the read happens |

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
    Complete(ctx context.Context, request CompletionRequest) (CompletionResult, error)
    Embed(ctx context.Context, texts []string) (EmbeddingResult, error)
    Capabilities() ProviderCapabilities
}
```

`Embed` answers a result rather than a bare `[][]float32` because a vector is only comparable with
others from the same model: an installation whose embedding model changes otherwise holds an index
with two geometries in it and a hybrid search that ranks the mixture by nothing. The model, the
dimensions and the moment travel with the batch, which is what lets the search notice and re-embed
([ADR-0049](../adr/ADR-0049-ai-provider-surface.md) decision 4).

Adapters: `OpenAiCompatible` (covers OpenAI, Azure, Mistral, vLLM, LiteLLM), `Ollama` (local),
`NoopAi` (the default). Configured per tenant, so that a provider can give its customers a choice.
Every adapter reaches its endpoint through `infrastructure/httpclient.GuardedClient` and none of
them brings a dependency — the wire is two JSON endpoints wide, and the timeout, the retry policy
and the SSRF refusal are the guarded client's (ADR-0049 decision 1). A refusal is always
`ErrUnavailable` with the detail code `ai.unavailable`, never an empty result: "the model said
nothing" and "there is no model" must not be the same value.

| Use case | Description | Result form |
|---|---|---|
| Jumble processing | Email/note → suggested title, due date, collection, labels, subtasks | Suggestion, confirmed by the user |
| Decomposition | Task → suggested work packages/activities | Suggestion |
| Classification | Suggested label/bucket, priority, duplicate detection | Suggestion |
| Summarisation | Comment thread, collection status, weekly review | Text (not persisted unless explicitly requested) |
| Semantic search | Embeddings of titles/notes in `pgvector`, hybrid search with `tsvector`. **Shipped in J-10**: one ranked page with one cursor, the words winning over the meaning, and pgvector detected rather than demanded ([ADR-0050](../adr/ADR-0050-pgvector-as-a-capability.md)) | Search result |
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
* **The one call somebody waits for is the search's**, and it is bounded rather than excepted
  (J-10). Embedding what somebody typed cannot be a job — the answer is wanted now — so it gets the
  shortest timeout in the product, 800 ms, and every way of not getting an answer is a **lexical
  search rather than an error**: no pgvector, no provider, no consent, an open circuit, a spent
  budget, or a provider that is merely slow. Nothing about it is on the write path: an entry's own
  embedding is maintained by a job seeded by the write, and an entry the job has not reached yet is
  found by its words in the meantime.
* Reproducibility: prompts are versioned resources (`infrastructure/ai/prompts/`), not inline in
  the code. **A prompt is never localised through the message catalogue**, and that is not an
  oversight of rule 8 but a consequence of versioning: a suggestion records the prompt version that
  produced it, so a text that depended on who was reading it would make that record unresolvable.
  Rule 8 forbids the backend producing what a person reads *in the product's interface*; a prompt is
  addressed to a model and to the client that operates one, exactly as a tool description is
  (J-12, `presentation/mcp/Prompts.go` states it where the code is).

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
