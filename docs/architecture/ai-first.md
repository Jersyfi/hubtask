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

**Enforced since J-14**, not merely written down. Until then `Descriptor.Destructive` fed
`destructiveHint` and stopped there, and nothing in the running system ever produced the actor kind
`AI_AGENT` at all — so the first two bullets were a description of an intention.

* Agents get **their own** service accounts with minimal scopes, never user credentials.
* Destructive operations (`purge`, `delete_container`, `empty_trash`, `delete_tenant`) are blocked
  by default for agent tokens and must be enabled explicitly. The permission is the scope
  `agent:destructive`, checked in `usecase.Registry.Invoke` — the single door all three channels
  come through — for every descriptor marked `Destructive`, so a use case added tomorrow is closed
  with nothing for anybody to remember. The refusal names the scope to set, because one an operator
  reads as a bug is one that gets worked around. **A rule cannot be used to launder it**: an agent
  writing an automation whose actions are destructive is refused at the moment of writing, since the
  rule would later run as an automation and the guardrail would not fire.
* **What makes a call an agent's is the door, not the credential.** Authentication answers `USER` or
  `SERVICE_ACCOUNT` from the account behind the token, which is the right answer to "who owns this"
  and the wrong one to "what is acting" — the same service account may drive a nightly import through
  REST and an agent through `/mcp`. So `presentation/mcp` stamps every call it serves as `AI_AGENT`,
  and a person calling `/mcp` with their own token is held to the agent's guardrails: the safe
  direction to be wrong in, and the only one that makes `destructiveHint` mean anything.
* Rate limits and quotas apply to agents just like to any other token — proved rather than
  assumed, and proved as *sharing a bucket*: a credential spending half its traffic through `/mcp`
  finds the same budget already spent. Neither the limiter nor the quota guard can see what kind of
  actor it is dealing with, which a gate holds them to.
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

**Every row says where it is.** This table was the product's intention for long enough that
`0.7.0` could report itself complete against it while three rows still described things nobody had
built - a suggested collection the code deliberately refuses, a `priority` the item model has never
had, and a summary of a comment thread nothing reads. J-17 walked QS-09 and brought `arc42.md`,
`roadmap.md` and `observability-reliability.md` current; this file was not on its list, because it
is the one a milestone reads rather than the one it reports into. So a row now names the task that
shipped it, the task that owes it, or the milestone it moved to, and a row that names none of the
three is a defect in this file rather than a plan.

| Use case | Description | Result form |
|---|---|---|
| Jumble processing | Email/note → suggested title, notes, due date, and the subtasks the material implies. **Shipped in J-06**, the subtasks in **K-01**: the payload carries the titles, and accepting converts the entry and then creates one child per title through the ordinary `CreateWorkItem` — J-07's walk over a flatter shape, a refusal partway leaving the entry and what stands under it. The notes and the due date were asked for from J-06 and applied by nothing until [#529](https://github.com/Jersyfi/hubtask/issues/529) in `0.8.0`: `ConvertJumbleEntry` declares neither, so the narrowing dropped both before the write, and the acceptance now writes them itself — the notes through `UpdateWorkItem`, the date through `SetDueDate` as an all-day date in the accepting person's zone. The material carries the current date, because a date "the material clearly implies" cannot be resolved against nothing. **Not labels**, since the same task: a label is a word a collection agreed on, an entry in the inbox is in none, and words a model invented instead would be vocabulary it invented — the entry is classified once it has been converted, which is where labels live. **Not a collection**: `Producing.go` filters `collection_id` out of every answer, because a model cannot know which collections a workspace has and one that names one is choosing a destination. The person converting supplies it | Suggestion, confirmed by the user |
| Field suggestions | An entry that already exists → a better title, notes, a due date. **Shipped in J-08** and given its own prompt in [#529](https://github.com/Jersyfi/hubtask/issues/529) (`0.8.0`): it asked the jumble's question until then, so it asked for labels nothing could apply and for subtasks nothing would grow — a work item is broken down by a decomposition, with a tree and types rather than a flat list. This row is new with that task; the table described the feature nowhere before it, which is the same defect in this file that `0.7.5` exists to end | Suggestion |
| Decomposition | Task → suggested work packages/activities. **Shipped in J-07**: the suggestion holds a tree rather than a field set, and accepting it is one ordinary `CreateWorkItem` per node in order, a refusal at the third leaving the two before it standing | Suggestion |
| Classification | Labels and the bucket — asked in **J-08**, and the answer made applicable in **K-02**: both are chosen from a set the provider was shown rather than named freely, so the filter above stays intact. The collection's vocabulary and the entry's board travel as the options with the entry's own column marked, an answer naming anything else is dropped, and accepting adds each label through `AddLabel` and moves the entry through `MoveWorkItem` — the ordinary use cases, with the accepting person's rights. Until K-02 the labels could be applied by nothing at all, so the narrowing dropped them and a classification recorded an empty payload, which is to say nothing. The values of the custom fields a workspace declared are **K-03**'s, the same shape a third time: the declarations travel with their kinds and their permitted values, the answer is checked by `ValidateValue` — the code `SetCustomField` itself runs — and accepting writes one key per call. Only closed kinds are offered (`SELECT`, `MULTI_SELECT`, `BOOL`); `TEXT`, `NUMBER`, `DATE` and `URL` are deliberately absent, because classifying into an open set is not classification, and `USER` because putting a person on an entry is naming rather than choosing. *"Priority"* is answered rather than built: this product has no such field and will not grow one to serve a prompt — a workspace that works with priority declares it, and the classifier fills what was declared. A workspace that declared none is unchanged, down to the number of provider calls it makes. Duplicate detection is **shipped in K-04**, and it is not a completion at all: the nearest neighbours of an entry's embedding, on the index J-10 maintains, at no token cost — `SuggestDuplicates` reads the index, refuses to call an entry's own branch a duplicate, narrows what it found to what the caller may read, and records identifiers rather than titles. The threshold is configuration (`HUBTASK_AI_DUPLICATE_THRESHOLD`). It is the one kind nothing accepts: what a person does about a duplicate is their decision through the ordinary use cases, and dismissing is what closes the proposal. No `pgvector`, no provider that can embed, or an entry the pass has not reached: each answers nothing at all, and the first two say so in `/meta/capabilities` before anybody asks | Suggestion |
| Summarisation | One entry's title and notes — **shipped in J-08**; the weekly review is an MCP prompt a client operates (J-12); the **comment thread and the collection's status shipped in K-05**. A thread is the entry's comments, oldest first and bounded at a hundred, fingerprinted against the *entry* so a reply does not make the summary stale; a collection's status is what is open, what moved and what is overdue, read from one level of it and bounded the same way. A comment travels as content and never as instruction, and nothing summarised — no comment, no title, no entry — reaches a log, a metric, a trace or an audit entry | A stored suggestion with its provenance (J-05), accepted into the entry or dismissed. This cell used to read *"text, not persisted"*; it was written before a suggestion was a record, and the record is the newer decision. A **collection's** summary is the exception nothing accepts: a collection has nowhere to put one, so it is read and dismissed |
| Semantic search | Embeddings of titles/notes in `pgvector`, hybrid search with `tsvector`. **Shipped in J-10**: one ranked page with one cursor, the words winning over the meaning, and pgvector detected rather than demanded ([ADR-0050](../adr/ADR-0050-pgvector-as-a-capability.md)). **The index holds 1536 dimensions, and which models fit it was undocumented until [#551](https://github.com/Jersyfi/hubtask/issues/551) in `0.8.0`**: a narrower model is stored exactly, zero-padded — cosine is the only distance the product uses, and padding changes neither the dot product nor either norm — and a wider one is refused at the first embedding with `ai.embedding_too_wide`, the job finishing rather than retrying and the search staying lexical ([ADR-0054](../adr/ADR-0054-embedding-width.md)). Until then the two Ollama embedding models in common use, 768 and 1024 wide, failed at insert with a raw database error, and semantic search was in effect OpenAI-only with nothing saying so | Search result |
| Template generation | A natural language description → `Template`. **Moved to `0.9.0`** by `0.7.0` decision 10: the least bounded of these seven and the least asked for, it goes beside the ecosystem work that gives it somewhere to come from | Draft |
| Translation | Item content on request. **Moved to `0.8.0`** by `0.7.0` decision 10: it is display-only and unpersisted, and the surface that would show two languages at once is the one that milestone builds | Display only, not persisted |

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
* **A model may pick from a set it was shown, and may not name a destination** (K-02). The filter
  that keeps `collection_id` out of every answer is about *naming*: a model cannot know which
  collections a workspace has, and one that names one is choosing a destination. A closed set is the
  other case — an entry's board columns, its collection's label vocabulary — small, already
  authorised, and read through the ordinary listing as the person who asked. Handed over as the
  options and validated on the way back, the answer is a choice from what was shown; an answer
  naming anything else is dropped and the rest of the suggestion stands. The options travel as
  content and never as instruction, because a column's name is something somebody typed.
  Where the set has validation of its own — a custom field's declaration — the check is *that*
  code rather than a copy of it (K-03): a value the suggestion keeps is one the acceptance can
  write, and a value it drops is one that would have been refused with a person's name on it.
* A prompt asks for exactly what the code keeps, and a **gate** says so
  (`test/architecture/promptanswers_test.go`, K-01). A prompt's answer shape is the fenced block it
  shows the model, the store reads the names out of it, and the allow list in `Producing.go` is
  compared against them in both directions. It is a gate rather than a rule because the defect is
  silent either way: `suggest-fields` asked for `subtasks` from J-06 to `0.7.5` and the allow list
  dropped the key, so every jumble suggestion paid a provider for an answer no code read. The allow
  list stays the authority — a declared key is one somebody decided the code may accept, and
  deriving it from the prompt text would let a prompt edit widen the filter.
* And a **second gate** says every key an allow list keeps is one the acceptance can actually apply
  (`test/architecture/applicablekeys_test.go`, [#529](https://github.com/Jersyfi/hubtask/issues/529)).
  A proposal is narrowed twice: once by the prompt's allow list, and again — since J-16 — to what
  the use case that would apply it declares, because the registry refuses an input a descriptor does
  not declare. K-01's gate cannot see the second narrowing, and between the two the leaks lived: a
  key kept and then dropped is a provider paid for an answer no code reads, and the drop is silent
  by construction, since dropping a key nobody declared is the filter working correctly. The gate
  reads the registry, the allow lists, the (prompt, target) declaration and the acceptance table,
  and it holds for the kinds that ask no provider too: `SuggestDuplicates` builds its payload in
  code, so no allow list narrows it, and a payload like that must never be merged into a use case's
  input.
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
