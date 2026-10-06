# AI First

"AI first" means two things here ([ADR-0012](../adr/ADR-0012-ai-first-mcp.md)):

1. **The application is operable by AI agents** exactly as it is by a human — completely, safely, traceably.
2. **AI features in the product** (suggestions, classification, semantic search) are an optional adapter, never a dependency of the core.

What it does not mean: AI in the domain model, or AI as a prerequisite for operation. Without an
AI provider, Hubtask remains fully functional (QS-09). Which AI features are built and how a
person uses them is in the use cases under [`docs/usecases/suggestion/`](../usecases/suggestion/).

---

## 1. The agent-facing interface

### 1.1 MCP server

`presentation/mcp` exposes the use case catalogue as **MCP tools** over streamable HTTP under
`/mcp`: `POST /mcp` carries the client's calls, `GET /mcp` the server's notifications, and
`Mcp-Session-Id` binds the two into one conversation. The tool list is generated from
`core/application/usecase.Registry` — every new use case is automatically available as a tool.

| Element | Rule |
|---|---|
| Tool name | The stable use case name in `snake_case` (`create_work_item`, `query_items`, `complete_work_item`) |
| Description | From the registry, including preconditions and side effects — agents need explicit semantics |
| Input schema | JSON Schema, identical to the REST request body |
| Auth | Service account or PAT with scopes; every tool call is audited as `actor.type = AI_AGENT` |
| Read/write marking | Tools carry `annotations.readOnlyHint` / `destructiveHint`, so that clients can ask for confirmation |
| Resources | Containers, items and views are also readable as MCP resources (`hubtask://items/{id}`). A resource is a read that is already in the catalogue, addressed by URI instead of by argument, so `resources/read` carries no authorisation of its own — exactly as `tools/call` carries none ([ADR-0051](../adr/ADR-0051-mcp-resources-are-catalogue-reads.md)). `resources/list` enumerates the hubs and the caller's saved views, paged; entries are reached by URI or found with `search_items`, because no unanchored item list exists |
| Session and stream | `initialize` hands out an `Mcp-Session-Id` and `GET /mcp` opens the server-initiated stream. The identifier is a **signed statement rather than a row**, so any pod checks it with no shared state. The stream is held to the *same* per-credential, per-tenant and per-process caps as `GET /stream`, in the same registry and the same metric |
| Prompts | Prepared MCP prompts, e.g. "weekly review from collection X", from the same versioned files under `infrastructure/ai/prompts/` the outbound adapters read — one store, so one prompt never exists in two versions. A prompt is published by describing itself; the prompts the product asks its own provider carry no title and stay unpublished. Arguments are declared and refused by name when unknown, and a resource argument becomes a link into the URI space above, so the permission is asked where the read happens |

### 1.2 Why the REST API is already agent-friendly

| Property | Benefit for agents |
|---|---|
| Idempotency (`Idempotency-Key`) | Retries after a timeout do not create duplicates |
| Optimistic locking (`ETag`/`If-Match`) | Prevents blindly overwriting concurrent changes |
| Machine-readable errors (`code` + `params`) | Self-correction instead of interpreting free text; a refused name is echoed back so the agent sees which |
| Capability manifest | The agent asks which fields and values are allowed instead of guessing |
| A query DSL instead of free-form search | Precise, checkable queries |
| Bulk operations | Few large steps instead of many individual calls |
| Dry run for automation | The agent can check the effect before executing |
| Complete audit | Every agent action is traceable and reversible |

### 1.3 Security guidelines for agents

* Agents get **their own** service accounts with minimal scopes, never user credentials.
* Destructive operations (`purge`, `delete_container`, `empty_trash`, `delete_tenant`) are blocked
  by default for agent tokens and must be enabled explicitly. The permission is the scope
  `agent:destructive`, checked in `usecase.Registry.Invoke` — the single door all three channels
  come through — for every descriptor marked `Destructive`, so a use case added later is closed
  with nothing to remember. The refusal names the scope to set. **A rule cannot launder it:** an
  agent writing an automation whose actions are destructive is refused at the moment of writing,
  since the rule would later run as an automation and the guardrail would not fire.
* **What makes a call an agent's is the door, not the credential.** Authentication answers `USER`
  or `SERVICE_ACCOUNT` from the account behind the token — the right answer to "who owns this" and
  the wrong one to "what is acting". So `presentation/mcp` stamps every call it serves as
  `AI_AGENT`, and a person calling `/mcp` with their own token is held to the agent's guardrails.
* Rate limits and quotas apply to agents as to any other token, and **share a bucket**: a
  credential spending half its traffic through `/mcp` finds the same budget already spent. Neither
  the limiter nor the quota guard can see what kind of actor it is dealing with, and a gate holds
  them to that.
* Content from items, comments, the jumble, inbound payloads and anything else a person or a
  system wrote is **data, not instructions**: prompt templates mark user content clearly as
  context, and server-side AI calls never carry out actions "demanded" in the text. Actions arise
  only from explicitly configured automation actions.
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
others from the same model: the model, the dimensions and the moment travel with the batch, which
lets the search notice a model change and re-embed ([ADR-0049](../adr/ADR-0049-ai-provider-surface.md)).

**Adapters:** `OpenAiCompatible` (covers OpenAI, Azure, Mistral, vLLM, LiteLLM), `Ollama` (local),
`NoopAi` (the default). A provider is configured per workspace (`/ai-provider`): kind, address,
models, a sealed key, the declared `jurisdiction`, and the separate consent `processing_allowed`.
Every adapter reaches its endpoint
through `infrastructure/httpclient.GuardedClient` and none brings a dependency — the timeout, the
retry policy and the SSRF refusal are the guarded client's. A refusal is always `ErrUnavailable`
with the detail code `ai.unavailable`, never an empty result: "the model said nothing" and "there
is no model" must not be the same value.

**Models the installation offers — decided, not yet built**
([ADR-0072](../adr/ADR-0072-ai-at-the-installation-level.md); UC-AI-05, UC-AI-06). An installation
may offer models as `ai_provider` rows with no workspace, written by operators and readable by a
workspace only as name, processor and jurisdiction. The instance value `ai.sources` (`NONE`,
`OFFERED`, `OWN`, `EITHER`; default `OWN`) says which sources a workspace may use; a lock can hold a
workspace out of a source, never in one, and a workspace may always choose `NONE`.
`ai.min_jurisdiction` bounds a workspace's own model. Switching on an offered model is the consent
act. The budget counts per source: `ai_tokens_per_day` for the offered source, an optional
`ai_own_tokens_per_day` for the workspace's own. A narrowed rule stops calls with
`ai.source_not_allowed` and deletes nothing. A change of the model behind search queues a
re-embedding in that workspace, seeded by the write that changed it. With no offered model, nothing
of this is visible.

**Every AI answer is a suggestion record**, except where the table says otherwise. A suggestion
carries its kind, its target, its payload and its provenance; accepting it performs the ordinary
use cases as the accepting person, with their rights and their audit entries; dismissing is a
state on the record ([domain-model.md](./domain-model.md) §5).

| Feature | Rule | Result form |
|---|---|---|
| Jumble processing | Email/note → suggested title, notes, due date, and the subtasks the material implies. Accepting converts the entry, writes the notes through `UpdateWorkItem` and the date through `SetDueDate` as an all-day date in the accepting person's zone, then creates one child per subtask title through `CreateWorkItem`; a refusal partway leaves what already stands. The material carries the current date, so an implied date can be resolved. **No labels** (an inbox entry is in no collection's vocabulary; it is classified after conversion) and **no collection** (`Producing.go` filters `collection_id` out of every answer; the person converting chooses it) | Suggestion, confirmed by the user |
| Field suggestions | An existing entry → a better title, notes, a due date, with a prompt of its own | Suggestion |
| Decomposition | Task → suggested work packages/activities as a tree; accepting is one `CreateWorkItem` per node in order, and a refusal at the third node leaves the two before it standing | Suggestion |
| Classification | Labels and the bucket, chosen from the options the provider was shown — the collection's label vocabulary and the entry's board columns with its own column marked; an answer naming anything else is dropped. Accepting adds each label through `AddLabel` and moves through `MoveWorkItem`. Custom field values are offered only for closed kinds (`SELECT`, `MULTI_SELECT`, `BOOL`), checked by `ValidateValue` — the code `SetCustomField` runs — and written one key per call; open kinds and `USER` are never classified. There is no built-in priority: a workspace that wants one declares a field, and the classifier fills what was declared. A workspace that declared none makes no extra provider call | Suggestion |
| Duplicates | The nearest neighbours of an entry's embedding on the semantic index, at no token cost. An entry's own branch is never a duplicate; what was found is narrowed to what the caller may read, and the record holds identifiers, never titles. The floor is `HUBTASK_AI_DUPLICATE_THRESHOLD`, default 0.85, measured ([evidence](../archive/evidence/K-04-2026-09-11.md)): a same-language paraphrase sits around 0.85–0.89, and a translation is not a duplicate to the measured models. Without pgvector, without a provider that embeds, or for an entry not yet embedded, it answers nothing; the first two are stated in `/meta/capabilities`. Nothing accepts it — what a person does about a duplicate goes through the ordinary use cases, and dismissing closes the proposal | Suggestion |
| Summarisation | One entry's title and notes; an entry's comment thread (oldest first, at most 100, fingerprinted against the entry so a reply does not make it stale); a collection's status (what is open, what moved, what is overdue, one level, at most 100 entries). Nothing summarised reaches a log, a metric, a trace or an audit entry. The weekly review is an MCP prompt a client operates | Suggestion accepted into the entry or dismissed; a collection's summary has nowhere to go and is only read and dismissed |
| Semantic search | Embeddings of titles/notes in `pgvector`, hybrid with `tsvector`: one ranked page with one cursor, the words winning over the meaning, pgvector detected rather than demanded ([ADR-0050](../adr/ADR-0050-pgvector-as-a-capability.md)). The index holds 1536 dimensions: a narrower model is stored zero-padded (cosine is unchanged by padding), a wider one is refused with `ai.embedding_too_wide` — before the first text is sent where the provider states its width, at the first batch where it does not — the job finishing rather than retrying, the search staying lexical, and `/meta/capabilities` and `/meta/health` saying so ([ADR-0054](../adr/ADR-0054-embedding-width.md)). An entry's embedding is maintained by a job seeded by its write | Search result |
| Template generation | A description in the caller's words and the target collection → a template in the contract's shape (a name, one root node with its tree; per node a type, title, optional notes, relative due offset). The material states which types may sit where, from the profiles `CreateTemplate` checks; a node the profile refuses is dropped with its subtree and counted (`dropped_nodes` on the suggestion, `nodes_dropped` in the job result); an offset `CreateTemplate` would refuse is left off. The words exist in no row, so they are held under row level security (`ai_request`) for the job and deleted when it ends. Accepting is `CreateTemplate` as the accepting person, the scope taken from the target, never from the answer | `TEMPLATE` suggestion |
| Translation | `AiTranslate` reads one entry's title and notes in a language the caller names (default: their own). It is a *read*: the entry's own read is the permission, consent and budget are asked after it, the call is bounded because a person is waiting, and every failure is `ai.unavailable`. Nothing is stored; the audit records the entry and the language, never the text ([i18n-l10n.md](./i18n-l10n.md) §7) | Display only, not persisted |

**Guardrails:**

* Results carry provenance (`source: AI`, model, timestamp, prompt version) and are marked as
  suggestions; accepting one is a regular user action with an audit entry.
* No automatic deletion or completion of items by AI without an explicit rule.
* **Absence is absence.** Where the manifest does not announce AI for the workspace, a client
  renders no AI control at all — no suggestion strip, no menu, no semantic toggle, no translate
  control — rather than a disabled one. A configured provider that is unreachable right now is
  different: the control is shown, gated with the reason `/meta/health` names.
* Data protection: AI use is opt-in per workspace; which fields are transmitted is documented; the
  self-hosting default is `NoopAi` or a local Ollama. A per-workspace `processing_allowed`
  field is checked before every call, and no installation or plan value sets it. Inside the workspace a
  person may keep their own content out, unless the workspace names a legal basis for AI as part
  of everybody's work ([data-protection.md](./data-protection.md) §4.1).
* Cost and latency: AI calls run as jobs, never in the write path, with timeouts, a token budget
  (`ai_tokens_per_day`, a quota row) and a circuit breaker for provider outages. The two calls a person waits
  for are bounded instead: a translation (above) and the search's embedding of what somebody
  typed, which has the shortest timeout in the product, 800 ms. Every way of not getting that
  embedding — no pgvector, no provider, no consent, an open circuit, a spent budget, a slow
  provider — is a **lexical search rather than an error**.
* **A model may pick from a set it was shown, and may not name a destination.** A model cannot
  know which collections a workspace has, so `collection_id` is filtered out of every answer. A
  closed set — an entry's board columns, its collection's labels — is small, already authorised,
  read through the ordinary listing as the person who asked, handed over as options and validated
  on the way back; an answer naming anything else is dropped and the rest of the suggestion
  stands. The options travel as content, never as instruction. Where the set has validation of
  its own (a custom field's declaration), the check is *that* code, not a copy of it.
* **A prompt asks for exactly what the code keeps.** A prompt's answer shape is the fenced block
  it shows the model; `test/architecture/promptanswers_test.go` compares the names in it with the
  allow list in `Producing.go` in both directions. The allow list stays the authority, so a prompt
  edit cannot widen the filter.
* **Every key an allow list keeps is one the acceptance can apply.** A proposal is narrowed twice —
  by the prompt's allow list, and to what the use case that applies it declares —
  and `test/architecture/applicablekeys_test.go` reads the registry, the allow lists, the
  (prompt, target) declaration and the acceptance table so that no key is paid for and silently
  dropped. A payload built in code (duplicates) is never merged into a use case's input.
* Reproducibility: prompts are versioned resources (`infrastructure/ai/prompts/`), not inline in
  the code. **A prompt is never localised through the message catalogue:** a suggestion records
  the prompt version that produced it, and a text that depended on the reader would make that
  record unresolvable. Rule 8 (no display text in the backend) is about the product's interface; a
  prompt is addressed to a model, as a tool description is (`presentation/mcp/Prompts.go`).

---

## 3. Looking ahead

| Expected development | Preparation today |
|---|---|
| Agents take over routine task maintenance | Full API parity, actor type `AI_AGENT`, granular scopes, audit |
| Several competing model providers | Provider behind a port, configured per workspace; offered models decided (ADR-0072) |
| Local models become the norm in self-hosting | The Ollama adapter |
| Agent protocols keep evolving | MCP is a presentation adapter — another protocol is another adapter, not a rebuild |
| Semantic search becomes expected | A `search` port with a lexical and a vector implementation |
| Traceability of AI decisions becomes a regulatory requirement | Provenance + audit + prompt versioning |
