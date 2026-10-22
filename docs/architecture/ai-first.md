# AI First

"AI first" means two things here ([ADR-0012](../adr/ADR-0012-ai-first-mcp.md)):

1. **The application is operable by AI agents** exactly as it is by a human — completely, safely, traceably.
2. **AI features in the product** (suggestions, classification, semantic search) are an optional adapter, never a dependency of the core.

It does not mean AI in the domain model or AI as a prerequisite: without an AI provider, Hubtask
remains fully functional (QS-09). Which AI features exist and how a person uses them is in
[`docs/usecases/suggestion/`](../usecases/suggestion/).

---

## 1. The agent-facing interface

### 1.1 MCP server

`presentation/mcp` exposes the use case catalogue as **MCP tools** over streamable HTTP under
`/mcp`: `POST /mcp` carries the client's calls, `GET /mcp` the server's notifications, and
`Mcp-Session-Id` binds them into one conversation. The tool list is generated from
`core/application/usecase.Registry`, so every new use case is a tool.

| Element | Rule |
|---|---|
| Tool name | The stable use case name in `snake_case` (`create_work_item`, `query_items`) |
| Description | From the registry, including preconditions and side effects |
| Input schema | JSON Schema, identical to the REST request body |
| Auth | Service account or PAT with scopes; every tool call is audited as `actor.type = AI_AGENT` |
| Read/write marking | `annotations.readOnlyHint` / `destructiveHint`, so clients can ask for confirmation |
| Resources | Containers, items and views are readable as MCP resources (`hubtask://items/{id}`). A resource is a catalogue read addressed by URI, so `resources/read` carries no authorisation of its own, as `tools/call` carries none ([ADR-0051](../adr/ADR-0051-mcp-resources-are-catalogue-reads.md)). `resources/list` pages the hubs and the caller's saved views; entries are reached by URI or `search_items`, since no unanchored item list exists |
| Session and stream | The `Mcp-Session-Id` from `initialize` is a **signed statement, not a row**, so any pod checks it without shared state. The `GET /mcp` stream is held to the same per-credential, per-tenant and per-process caps as `GET /stream`, in the same registry and metric |
| Prompts | Prepared MCP prompts (e.g. "weekly review from collection X") from the same versioned files under `infrastructure/ai/prompts/` the outbound adapters read, so no prompt exists in two versions. A prompt is published by describing itself; the product's own prompts carry no title and stay unpublished. Unknown arguments are refused by name, and a resource argument becomes a link into the URI space, so permission is asked where the read happens |

### 1.2 Why the REST API is already agent-friendly

| Property | Benefit for agents |
|---|---|
| Idempotency (`Idempotency-Key`) | Retries after a timeout do not create duplicates |
| Optimistic locking (`ETag`/`If-Match`) | Prevents blindly overwriting concurrent changes |
| Machine-readable errors (`code` + `params`) | Self-correction instead of interpreting free text; a refused name is echoed back |
| Capability manifest | The agent asks which fields and values are allowed instead of guessing |
| A query DSL instead of free-form search | Precise, checkable queries |
| Bulk operations | Few large steps instead of many individual calls |
| Dry run for automation | The agent can check the effect before executing |
| Complete audit | Every agent action is traceable and reversible |

### 1.3 Security guidelines for agents

* Agents get **their own** service accounts with minimal scopes, never user credentials.
* Destructive operations (`purge`, `delete_container`, `empty_trash`, `delete_tenant`) are blocked
  for agent tokens unless the scope `agent:destructive` is granted. It is checked in
  `usecase.Registry.Invoke` — the single door of all three channels — for every descriptor marked
  `Destructive`, so a later use case is closed too; the refusal names the scope. **A rule cannot
  launder it:** an agent writing an automation with destructive actions is refused at the write.
* **What makes a call an agent's is the door, not the credential.** Authentication answers `USER` or
  `SERVICE_ACCOUNT` from the token's account; `presentation/mcp` stamps every call `AI_AGENT`, so a
  person calling `/mcp` with their own token is held to the agent's guardrails.
* Rate limits and quotas apply to agents as to any token and **share a bucket** with the credential's
  other traffic; a gate holds the limiter and quota guard blind to the actor type.
* Content from items, comments, the jumble, inbound payloads and anything else a person or system
  wrote is **data, not instructions**: prompt templates mark it as context, and server-side AI calls
  never carry out actions it demands. Actions arise only from configured automation actions.
* An agent cannot create a rule with more rights than the agent itself.

---

## 2. AI features in the product

Behind the `Provider` port in `core/port/ai/Port.go` (`Complete`, `Embed`, `Capabilities`). `Embed`
answers the model, the dimensions and the moment with the vectors, because a vector is comparable
only within one model and search must notice a model change and re-embed
([ADR-0049](../adr/ADR-0049-ai-provider-surface.md)).

**Adapters:** `OpenAiCompatible` (OpenAI, Azure, Mistral, vLLM, LiteLLM), `Ollama` (local), `NoopAi`
(the default). A provider is configured per workspace (`/ai-provider`): kind, address, models, a
sealed key, the declared `jurisdiction`, and the separate consent `processing_allowed`. Every adapter
uses `infrastructure/httpclient.GuardedClient` (timeout, retry, SSRF refusal) and brings no
dependency. A refusal is always `ErrUnavailable` with `ai.unavailable`, never an empty result: "the
model said nothing" and "there is no model" must differ. A spent budget is the same refusal with the
parameters `reason` (`budget_spent`) and `renews_at`, so the person reads when it renews (UC-AI-07
check 3); a provider that refuses says so before it is called (`ai.Refusing`), so a caller that
short-cuts on its capabilities passes the same refusal on. An ask answers `202` with the job it
queued; a refusal the job meets after that - the budget spent since, AI switched off - ends the job
`FAILED` with the refusal's code and parameters (`error_code`, `error_params`), not in the dead
letter, and the client that follows the job says it the way it says the refusal on a request. A
question that runs to its end ends the job `SUCCEEDED` with its outcome as a message code
(`outcome_code`): `ai.proposed`; `ai.nothing_to_change` (the answer proposed nothing, or could not
be read); `ai.nothing_to_work_with`, `ai.kept_out` or `ai.processing_restricted` (the same codes the
ask refuses with up front, met after queueing). The client ends its pending state on every end, says
after about a minute that the AI takes longer than usual while it keeps following (paused while the
page is hidden), and does not offer a question `workable` would refuse on facts the asker may know:
`classify` with no label, no column and no closed field to choose from is refused
`ai.nothing_to_choose_from`.

**Models the installation offers** ([ADR-0072](../adr/ADR-0072-ai-at-the-installation-level.md);
UC-AI-05, UC-AI-06). Operators offer models as `ai_provider` rows with no workspace
(`/admin/ai-providers`, `hubctl admin ai`, the instance file's `ai_models` with each key named by
an environment variable). Every workspace transaction reads them for the call - the read policy
admits them, the write policies only in the installation's own scope - and every statement about
a workspace's own row names its workspace. A workspace is *answered* an offered model's name,
processor and jurisdiction only (`/ai-source`). `ai.sources` (`NONE`, `OFFERED`, `OWN`, `EITHER`;
default `OWN`) and `ai.min_jurisdiction` (`ANY` < `ADEQUACY` < `EEA` < `SELF_HOSTED`) are the `ai`
area of the instance level, each with a lock; a workspace narrows `ai.sources` in its archived
settings where nothing is locked, and `NONE` is always its own - a lock holds a workspace out of a
source, never in. The workspace's choice (`ai_source`: `OWN`, `OFFERED` with the model, or `NONE`) is
outside the archive: choosing an offered model is the consent act, choosing its own or none
withdraws it; the own model's consent stays `processing_allowed`. The application selects what may
be called (`AiRules.Select`) and the resolver builds the adapter for it; a source the rule no
longer allows, an own model below the minimum or a withdrawn offered model answers
`ai.source_not_allowed` on every call and deletes nothing. An offered model's vectors are stored
under `offered:<id>/<model>`, so an own model of the same name is another space; a source switch or
a changed own embedding model seeds the embedding pass in the same transaction, an operator's change
to an offered model is picked up by each workspace's repeating pass, and `/ai-source` answers
`search_rebuilding` while another model's vectors remain. Budgets count per source, each on the
workspace's own day: `ai_tokens_per_day` (offered, the installation's), optional
`ai_own_tokens_per_day` (own, the workspace's) — the resolver says which source answered and the
budget wrapper checks and meters that one ([multi-tenancy.md](./multi-tenancy.md) §4). An offered
model's key is re-sealed by the installation's own round of a key rotation
([security.md](./security.md) §8.1). With no offered model, none of this is visible.

**Whose content stays out** ([data-protection.md](./data-protection.md) §4.1; UC-PRV-05). One
exclusion, read in the same transaction as the material, decides it for every path that hands
content to a model - the suggestion producer, translation and the embedding pass - and takes its
reasons from sources: a person's objection (their newest `ai_processing` consent record is a
withdrawal, the workspace offers keeping one's content out, and more than one person works there)
is one; a restriction of processing (Art. 18, data-protection.md §4) is the other, read first and
without the objection's gates - it holds whatever the workspace's position and for a person alone,
and it also keeps out an entry assigned to the person. What a kept-out person created, their
comments and what they mailed in are not material; their display name, its parts of three letters
or more, their sign-in name and their address become `[person]` in what remains. Material with
nothing left is refused at the asking with `ai.nothing_to_work_with` - the words an empty one gets,
so no answer to a colleague names or implies who objected - and the person themselves is told
their own reason, `ai.kept_out` or `ai.processing_restricted`; the manifest answers `ai_suggestions` and `semantic_search` false for them, and they
search by words. The embedding pass skips their entries and forgets their vectors; recording the
objection or the restriction forgets them in its own transaction. A rule's AI action with nothing to work with is
skipped, not failed.

**Every AI answer is a suggestion record** unless the table says otherwise: kind, target, payload and
provenance. Accepting performs the ordinary use cases as the accepting person, with their rights and
audit entries; dismissing is a state on the record ([domain-model.md](./domain-model.md) §5).

**Who may ask, and where** (UC-AI-04 checks 6–9). A request about an entry - an ask, the
listing, an acceptance, a dismissal - reads the entry first and asks its permission along the
entry's own path, so a role held on its hub, its collection or the entry itself counts and
"assigned only" is measured against its stored assignee; a request about a collection asks along
the collection's path. Asking and deciding ask what changing the entry asks; the listing and the
two **reading aids** - a translation and a summary of the discussion - ask what reading it asks,
so everybody who may read the entry is offered them; a summary applied to the notes asks what
changing asks. A field suggestion may name the fields it asks about (`fields`); the producer keeps
only those. The person who asked a question may watch and stop its job (`GET /jobs/{id}`,
`:cancel`) as long as they may still read what it is about; a question stopped before its answer
is recorded records nothing (the producer holds the job's row in that transaction), one stopped
after leaves its proposal standing, and a stopped job ends without a failure. `GET
/ai-budget-state` answers every signed-in person whether the chosen source can still be asked
today - `OK`, `NEARLY_SPENT` (more than nine tenths used, A-18's condition), `SPENT` with when it
renews, or `OFF` - and no number. A summary of a discussion is judged against the entry's title and
notes, not the thread, so a reply does not mark it stale: it says what the discussion came to when
it was made, and a writer dismisses it to ask again.

| Feature | Rule | Result form |
|---|---|---|
| Jumble processing | Email/note → suggested title, notes, due date, implied subtasks and the collection it fits best; the material carries the current date and the shared set of collections (the guardrail below). Accepting converts the entry into the proposed collection unless the person picks another, writes notes (`UpdateWorkItem`), the date as all-day in the accepter's zone (`SetDueDate`) and one child per subtask (`CreateWorkItem`); a refusal partway leaves what stands. **No labels** (no collection vocabulary yet). **On arrival** (UC-JUM-07 check 10): where the workspace switched it on (`PUT /ai-arrival-proposals`, its own, off by default, set by no level above it), a subscriber of `jumble.entry.received` queues one `ai.suggest_arrival` job per arrival and nothing else; the job asks as the system for the workspace - the embedding pass's actor, the switch being the authorisation - and judges at its run, each with its outcome: the switch (`ai.arrival_proposals_off`), the settlement, whose content stays out (the sender address, the submitting account, the principal an agent or a rule acted for), the source and its consent, `ai_arrival_proposals_per_day` (`ai.arrival_cap_reached`) and the token budget. Nothing counts or announces waiting proposals | Suggestion, confirmed by the user |
| Field suggestions | An existing entry → a better title, notes, a due date, with its own prompt | Suggestion |
| Decomposition | Task → work packages/activities as a tree; accepting is one `CreateWorkItem` per node in order, and a refusal leaves the nodes before it standing | Suggestion |
| Classification | Labels and bucket, chosen from the options shown (the collection's labels, the entry's board columns with its own marked); anything else is dropped. Accepting is `AddLabel` per label and `MoveWorkItem`. Custom field values only for closed kinds (`SELECT`, `MULTI_SELECT`, `BOOL`), checked by `ValidateValue` and written one key per call; open kinds and `USER` never. No built-in priority: the classifier fills declared fields, and a workspace with none makes no extra call | Suggestion |
| Duplicates | The nearest neighbours of an entry's embedding, at no token cost. Never its own branch; narrowed to what the caller may read; identifiers only, never titles. Floor `HUBTASK_AI_DUPLICATE_THRESHOLD`, default 0.85, measured ([evidence](../archive/evidence/K-04-2026-09-11.md)): a paraphrase scores about 0.85–0.89, a translation is not a duplicate. Without pgvector, an embedding provider (both stated in `/meta/capabilities`) or the entry's embedding, it answers nothing. Nothing accepts it; dismissing closes it. **Before an entry exists** (`POST /items:duplicates`, `FindDuplicatesByTitle`): the typed title is embedded as a search's words are (consent, kept-out rule, the 800 ms wait), compared at the same floor, at most five, each narrowed to what the caller may read along its own path, answered with titles to the asker alone and stored nowhere, not audited. The floor was measured entry against entry; a typed title against stored title-and-notes vectors scores lower, so it finds fewer rather than more | Suggestion; a read while creating |
| Summarisation | One entry's title and notes; its comment thread (oldest first, at most 100, fingerprinted against the entry so a reply does not make it stale); a collection's status (open, moved, overdue; one level, at most 100 entries). Nothing summarised reaches a log, metric, trace or audit entry. The weekly review is an MCP prompt | Suggestion accepted into the entry or dismissed; a collection's summary is only read and dismissed |
| Semantic search | Embeddings of titles/notes in `pgvector`, hybrid with `tsvector`: one ranked page, one cursor, words winning over meaning, pgvector detected rather than demanded ([ADR-0050](../adr/ADR-0050-pgvector-as-a-capability.md)). The index holds 1536 dimensions: narrower is zero-padded (cosine unchanged), wider is refused with `ai.embedding_too_wide` — before the first text where the provider states its width, else at the first batch — the job finishing, search staying lexical, `/meta/capabilities` and `/meta/health` saying so ([ADR-0054](../adr/ADR-0054-embedding-width.md)). A job seeded by the entry's write maintains its embedding | Search result |
| Template generation | The caller's description and target collection → a template in the contract's shape. The material states which types may sit where (the profiles `CreateTemplate` checks); a refused node is dropped with its subtree and counted (`dropped_nodes`, `nodes_dropped`), a refused offset left off. The words are held under row level security (`ai_request`) for the job and deleted when it ends. Accepting is `CreateTemplate` as the accepter, scope from the target, never from the answer | `TEMPLATE` suggestion |
| Rule drafting | `DescribeRule` takes a sentence and the scope the editor holds, and asks what writing a rule asks: `AUTOMATION` at the scope, `automation:manage`, a plan with rules (automation.md §2.1). The material is the vocabulary a draft may use - an allowlist of entry-level action kinds, each field classified (`draftableActions`, held to the catalogue by a test), the trigger kinds but `SCHEDULE`, the composer's condition subjects but board columns and custom fields - and the people, collections, labels and places the sentence names that the asker may see, at most twenty each; whoever keeps their content out is never named. The answer is narrowed to that: what was not shown is dropped and named in `not_drafted` (a message code, or the person's own words quoted - a quote that is not in their sentence is dropped). The words are held in `ai_request` for the job, which runs as `ai.draft_rule`. The draft is about the asker's own `ACCOUNT`: only they read, accept or dismiss it. Accepting is `CreateRule` as the asker with `name` and `run_as` in the overrides, so the rule is stored switched off; a draft is never applied without a person | `RULE` suggestion |
| Translation | `AiTranslate` reads one entry's title and notes in a named language (default: the caller's). A *read*: the entry's read is the permission, consent and budget are asked after, the call is bounded because a person waits, every failure is `ai.unavailable`. Nothing is stored; the audit records entry and language, never text ([i18n-l10n.md](./i18n-l10n.md) §7) | Display only, not persisted |

**Guardrails:**

* Results carry provenance (`source: AI`, model, timestamp, prompt version) and are marked as
  suggestions; accepting is a regular, audited user action.
* No automatic deletion or completion of items by AI without an explicit rule.
* **Absence is absence.** Where the manifest does not announce AI for the workspace, a client renders
  no AI control at all, not a disabled one. A configured provider that is unreachable is shown,
  gated with the reason `/meta/health` names.
* Data protection: AI is opt-in per workspace; the transmitted fields are documented; the
  self-hosting default is `NoopAi` or a local Ollama. `processing_allowed` is checked before every
  call and set by no installation or plan value. A person may keep their own content out unless the
  workspace names a legal basis ([data-protection.md](./data-protection.md) §4.1).
* Cost and latency: AI calls run as jobs, never in the write path, with timeouts, a token budget
  (`ai_tokens_per_day`, a quota row) and a circuit breaker. The two calls a person waits for are
  bounded instead: a translation and the search's embedding of the typed words (800 ms, the
  product's shortest timeout). Any failure to get that embedding is a **lexical search, not an
  error**.
* **A model may pick from a set it was shown, never name a destination.** `collection_id` is
  filtered from every answer, except a jumble proposal's destination (decision #1629): there the
  set passed is the **shared set** — the collections everybody who may convert an arrival may
  write in: not trashed, not archived by their own stamp or their hub's, outside private hubs, the
  first hundred in the tree's order — whoever asks, so that no proposal another reader sees names
  a private hub's collection, and the answer is validated against it like any other closed set. A
  listing answers a proposed destination only while it is still in that set. Every other closed
  set (board columns, labels) is read through the ordinary listing as the asking person, passed as
  options — content, never instruction — and validated on return; anything else is dropped and the
  rest stands. Where the set has its own validation (a custom field's declaration), the check is
  that code, not a copy.
* **A model writes no expression.** A condition a model drafts is answered as the rule editor's
  sentences and compiled by the server's twin of the composer (`core/domain/model/automation`), so
  no byte of an answer becomes expression text except as a literal the compiler escaped - rule 9's
  reasoning, applied to the rules' language. Branch conditions, a dedupe key and an outbound body
  are never drafted, and no draft names an address.
* **A prompt asks for exactly what the code keeps.** `test/architecture/promptanswers_test.go`
  compares a prompt's fenced answer shape with the allow list in `Producing.go` both ways; the allow
  list stays the authority.
* **Every key an allow list keeps is one the acceptance can apply** - or one it only tells the
  person (`toldKeys`: what a rule draft could not draft). A proposal is narrowed by the
  allow list and by what the applying use case declares; `test/architecture/applicablekeys_test.go`
  holds registry, allow lists, (prompt, target) declarations and acceptance table together. A
  payload built in code (duplicates) is never merged into a use case's input.
* Reproducibility: prompts are versioned resources (`infrastructure/ai/prompts/`). **A prompt is
  never localised through the message catalogue**: a suggestion records its prompt version, and a
  reader-dependent text would make that unresolvable. Rule 8 concerns the product's interface; a
  prompt addresses a model, as a tool description does (`presentation/mcp/Prompts.go`).

---

## 3. Looking ahead

| Expected development | Preparation today |
|---|---|
| Agents take over routine task maintenance | Full API parity, actor type `AI_AGENT`, granular scopes, audit |
| Several competing model providers | Provider behind a port, configured per workspace or offered by the installation (ADR-0072) |
| Local models become the norm in self-hosting | The Ollama adapter |
| Agent protocols keep evolving | MCP is a presentation adapter — another protocol is another adapter, not a rebuild |
| Semantic search becomes expected | A `search` port with a lexical and a vector implementation |
| Traceability of AI decisions becomes a regulatory requirement | Provenance + audit + prompt versioning |
