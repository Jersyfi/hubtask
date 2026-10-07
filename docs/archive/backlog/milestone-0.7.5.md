# Milestone 0.7.5 — What §2 promised

The goal: `ai-first.md` §2's table stops describing an intention and starts describing the product.
`0.7.0` built the port, the adapters, the record, the search and the agent-facing half, and it
proved the hardest thing it had to prove — that the product is undiminished with none of it
switched on. What it did not do is notice that three of the five rows it kept say more than what
landed under them, and that a fourth asks a provider for something the code throws away. The rows
are the vision as it is written down; a row nobody built is a promise to whoever reads it next,
which for §2 is `F5` — the milestone that renders every one of these.

So this milestone is small, unheroic and entirely about the difference between a table and a
product. Four rows get the part of them that is missing; one route gets the door
`0.3.5`'s own acceptance said it had; and one test makes the class of defect that produced K-01
impossible to introduce again.

`0.7.5` rather than `0.7.1`, because `versioning-release.md` §2's table gives PATCH to "a bug fix
without a contract change" and every task here but one adds a use case or a scope. `.5` is the shape
this project already uses for a minor that was inserted rather than planned (`0.3.5`, `0.4.5`).

Every task is one pull request. Nothing here depends on anything else here, which is the one
pleasant property of a milestone made of leftovers.

Legend: **[L]** = best done locally with Claude Code (you see every step),
**[G]** = delegable through a GitHub issue.

What deliberately is **not** in this milestone: **a `priority` field**. §2's Classification row names
one, the item model has never had one, and adding a field to the item model in order to satisfy a
prompt is the wrong direction of travel — K-03 says what the row means instead. **Translation** and
**template generation** stay where `0.7.0` moved them (`0.8.0` and `0.9.0`); this milestone only
writes down in `ai-first.md` that they moved, because until now only `roadmap.md` said so. And every
**screen** that renders any of this remains `F5`'s, one window behind, exactly as before.

Five decisions taken while writing this backlog, so that nobody re-derives them:

* **The letter is K.** J was `0.7.0`, and this milestone is not part of it: a task numbered `J-18`
  would sit in a closed milestone's list and read, for years, as something that was in scope and
  was missed. It was not missed — it was never built, which is a different sentence and deserves
  its own letter.
* **A model may pick from a set it was shown, and may not name a destination.** `Producing.go`
  filters `collection_id` out of every answer with the reason written beside it: *"a model cannot
  know which collections a workspace has"*. That reasoning is about *naming* — it does not forbid
  *choosing*. A container's buckets are a closed, small, already-authorised set; handed to the
  provider as the options and validated on the way back against the same set, a bucket is a choice
  rather than a destination the model invented. K-02 and K-03 are both that shape, and neither of
  them loosens the filter: `collection_id` stays out, because the set it would be chosen from is
  the workspace's whole structure.
* **"Priority" is a custom field, not a new column.** A workspace that works with priority declares
  it — usually a `SELECT` with its own options, which is exactly the closed set the decision above
  needs. So K-03 proposes *values for the fields the workspace declared*, which serves the row for
  the workspaces that meant it and adds nothing to the item model for the ones that did not. This
  is the answer to a row that has been in the document since before custom fields existed.
* **Duplicate detection spends no tokens.** It is the one §2 entry that is not a completion: two
  entries are near each other or they are not, and J-10 already stores the vector that says so. So
  K-04 is a query with a threshold, it works on an installation whose provider can embed and cannot
  complete, and it produces nothing at all where there is no `pgvector` — the third of J-10's three
  degradations, reused rather than reinvented.
* **The health report is the one non-AI task here, and it is here because it has nowhere else to
  go.** `#507` is a defect of `0.3.5`, found by `0.7.0`'s QS-09 walk, and blocking a component `F1`
  already built. A milestone that exists to close the gap between what a document says and what the
  product does is the right home for it.

---

## K-01 — The subtasks the entry implies **[L]**

*Depends on: nothing.*

`infrastructure/ai/prompts/suggest-fields.v1.md` asks the provider, in its own words, for
`subtasks`: *"up to ten short titles, only where the material describes separable pieces of work"*.
`promptFields["suggest-fields"]` in `core/application/service/suggestion/Producing.go` allows
`title`, `notes`, `due_date` and `labels`. The key is dropped, silently, by the security filter that
is right to drop keys nobody declared — so every jumble suggestion since J-06 has paid a provider
for an answer no code reads, and §2's Jumble row and J-06's own acceptance have both named a field
that never arrives.

Two things to fix and they are not the same size. The small one is the leak: a prompt that asks for
a key the allow list drops is a defect the compiler cannot see, so this task ends it with a **test
that reads both** — every answer key a prompt file documents is in that prompt's allow list, and
every key in the allow list is asked for. It is the cheapest gate in this milestone and the reason
K-01 goes first.

The large one is what a kept `subtasks` means. A field set is not a tree, and J-07 already built the
tree: `KindDecomposition`, `keptTree`, and an acceptance that walks it creating one entry per node
with the accepting person's rights, reporting what was created when part of it is refused. This
backlog proposes, and the task confirms or replaces with a reason: the jumble's question keeps its
one call and its one suggestion, the payload carries the titles, and **acceptance reuses J-07's
walk** — `ConvertJumbleEntry` creates the entry, and each kept title becomes a child through
`CreateWorkItem` under it, in order, with the ordering keys `Ordering.go` produces. A refusal at the
third child leaves the entry and the first two standing and says so, because that is already how a
partial acceptance behaves and a second answer to the same question would be the wrong kind of
novelty.

**Acceptance:** no prompt file asks for a key its allow list drops and no allow list holds a key no
prompt asks for, proved by a test that reads the prompt store and the map rather than by review; a
jumble entry whose material describes separable work produces a suggestion whose payload carries the
titles, and one that does not produces the same suggestion without them; accepting creates the entry
and its children through the ordinary use cases with the accepting person's rights, in order, and
reports what was created when part of it is refused; a child the domain refuses — a level the
capability profile does not allow, a title too long — leaves everything before it standing; the
levels obey `domain-model.md` §2 and the ordering uses `Ordering.go`; the injection test gains the
case where a subtask title is an instruction; cross-tenant negative test; §2's Jumble row is current;
`make verify` is green.

**Read:** `core/application/service/suggestion/Producing.go` (the allow list and its reasoning);
J-06 and J-07 in `milestone-0.7.0.md`; `domain-model.md` §2, §5; `ai-first.md` §1.3, §2;
ADR-0049 decision 3 (the prompt store and its versions)

---

## K-02 — The bucket, chosen from the buckets that exist **[G]**

*Depends on: nothing.*

§2's Classification row says "label/**bucket**" and `promptFields["classify"]` allows `labels`.

The reason it allows only labels is a good one and it does not apply here. `collection_id` is
filtered out because a model cannot know which collections a workspace has, and a model that names
one is a model choosing a destination. A bucket is the other case: the entry is already in a
container, that container has the buckets it has, and the set is small, closed and already
authorised for the person asking. Handed to the provider as the options — by name, with the entry's
current one marked — and validated on the way back against the same set, the answer is a choice from
what it was shown. An answer naming anything else is dropped exactly the way an undeclared key is.

The prompt is a new version rather than an edit: `classify.v2.md`, with `v1` staying where it is, so
that a suggestion recorded last week still resolves to the words that produced it (ADR-0049
decision 3). Accepting sets the bucket through `MoveWorkItem` — the ordinary use case, the ordinary
permission check, the ordinary activity entry — not through a second write path.

**Acceptance:** the prompt is a new version and the old one stays; the buckets travel as options and
an answer naming a bucket that was not offered is dropped with the suggestion still stored for its
labels; the payload's `bucket_id` reaches the allow list and nothing else does; accepting moves the
entry through the ordinary use case as the accepting person and is refused when they may not move
it; an entry in a container with no buckets produces a suggestion with labels and no bucket, not an
error; the injection test covers a bucket name that reads as an instruction; cross-tenant negative
test; §2's Classification row is current; `make verify` is green.

**Read:** `core/application/service/suggestion/Producing.go`; `domain-model.md` §2 (buckets);
`core/application/service/work/MoveWorkItem.go`; `ai-first.md` §1.3, §2; ADR-0049

---

## K-03 — The declared fields' values, which is where "priority" lives **[G]**

*Depends on: K-02 (the same shape, and it should land second so the pattern is one).*

§2's Classification row has said "priority" since before this repository had custom fields, and the
item model has never had such a column. Building one now would be a field added to the domain in
order to satisfy a prompt, which is the wrong direction of travel and the kind of change
`CLAUDE.md` says not to make in passing.

What the product has is `DefineCustomField` and `SetCustomField`: a workspace that works with
priority declares it, usually as a `SELECT` with its own options, in its own words, in its own
language. That declaration is exactly the closed set K-02's decision needs — so the classifier
proposes **values for the custom fields the container declares**, with the declaration travelling as
the options and the answer validated against it. A workspace that declared none gets what it gets
today. A workspace that declared four gets four questions answered in one call, and "priority" is
one of them if that is what they called it.

The validation is the declaration's own, not a second copy: a value that `SetCustomField` would
refuse is one this must refuse, and the way to be sure of that is to ask the same code. A field kind
that has no closed set — free text, a number — is out of scope here and the task says so rather than
letting a model write into it: the row says "classification", and classifying into an open set is
not classification.

**Acceptance:** the declared fields of the entry's container travel to the provider as options, with
their kinds and their allowed values, and no field's *stored value* travels unless it is the entry's
own; an answer naming a field the container has not declared, or a value the declaration does not
allow, is dropped and the rest of the suggestion is kept; only closed-set kinds are offered and the
open ones are documented as deliberately absent; accepting writes through `SetCustomField` as the
accepting person with the ordinary permission check and the ordinary activity entry; a workspace
with no declared fields is unchanged in every observable way, including the number of provider calls
it makes; cross-tenant negative test; §2's Classification row is current and no longer says
"priority" without saying what that means; `make verify` is green.

**Read:** `core/application/service/work/DefineCustomField.go` and `SetCustomField.go`;
`domain-model.md` §2 (custom fields); K-02 above; `ai-first.md` §2; `api-guidelines.md` §6

---

## K-04 — Duplicates, on the index J-10 built **[L]**

*Depends on: nothing, and it is the only task here that does more where `pgvector` exists.*

The last third of §2's Classification row, and the one entry in the whole table that is not a
completion. Two entries are near each other in the embedding space or they are not; J-10 already
maintains the vector that says so, by a job seeded by the write, with the store detected rather than
demanded (ADR-0050).

So this is a query and a threshold, not a prompt. For one entry: its nearest neighbours above a
similarity floor, excluding itself, excluding what it is already a child or a parent of, narrowed to
what the actor may see **after** the read for the reason `SearchItems` narrows there — the rows come
from everywhere at once. The result is a suggestion of its own kind, because accepting it is neither
a field write nor a tree: what a person does with a duplicate is decide, and the two answers are
"dismiss" and "these are the same" — the second being a link a person makes with the ordinary use
cases, which this task names rather than invents.

Three states, all of them answers: no `pgvector`, so no suggestion and `/meta/capabilities` already
says why; a provider that cannot embed, the same; an entry the embedding job has not reached, which
is found by nobody and finds nobody until it has been — the correct degradation rather than a gap,
and the same one J-10 established.

The threshold is a number somebody has to choose, and a number chosen by a session is a number
nobody can defend later. So it is configuration with a documented default, the default is measured
against a real corpus in the pull request, and the measurement is what the description points at.

**Acceptance:** an entry that is a near-duplicate of another is proposed as one, and two entries
that merely share a word are not; the narrowing to what the actor may see is proved by a
cross-tenant negative test and by a test with two collections and one reader; the parent and the
children of an entry are never proposed as its duplicates; an installation without `pgvector`, one
without an embedding provider, and an entry not yet embedded each produce no suggestion and no
error, and the first two are visible in `/meta/capabilities`; the threshold is configuration with a
default the pull request measured; no provider call is made and no budget is spent, proved rather
than asserted; the use case is in `domain-model.md` §5, registered, and green in the parity test;
message codes in `locales/en.json`; §2's Classification row is current.

**Read:** `core/application/service/work/SearchMeaning.go` and `Embedding.go` (J-10);
ADR-0050; `domain-model.md` §5; `ai-first.md` §2; `api-guidelines.md` §4, §6; J-05 in
`milestone-0.7.0.md`

---

## K-05 — Summarising a thread, and a collection **[G]**

*Depends on: nothing.*

§2's Summarisation row names three things — "comment thread, collection status, weekly review" — and
`0.7.0` built one third of one of them: `AiSummarize` reads an entry's title and its notes. The
weekly review exists, as an MCP prompt a client operates (J-12), and that is the row's third item
answered in the place it belongs. The other two are the product's own and are missing.

A comment thread is the same target and different material: the entry's comments, in order, oldest
first, bounded — a thread of four hundred comments is not a summary problem, it is a token problem,
and the bound is part of the design rather than a surprise at the provider. A collection's status is
a new target type, `CONTAINER`, which the suggestion aggregate takes the way it took `JUMBLE_ENTRY`:
what is open, what moved, what is overdue, in the shape a person asks a colleague on a Monday.

Rule 10 is the one to watch and it is easy to break twice here. A comment is user content, so it
travels as fenced context and never as instruction, and the injection test gains a comment-shaped
case. And nothing about either summary — no title, no comment, no collection name — reaches a log, a
metric, a trace or an audit entry; what the audit records is that somebody asked, about what, with
which prompt, which is already the shape `ai.summary_asked` has.

The result stays what J-05 made it: a stored suggestion with its provenance, which somebody accepts
into the entry's notes or dismisses. §2's "Text (not persisted unless explicitly requested)" was
written before there was a suggestion record and is the older sentence; the row says so in this
task rather than the code growing a second, unrecorded path to satisfy it.

**Acceptance:** a thread summary reads the entry's comments in order with a documented bound and
says in its own suggestion which prompt produced it; a collection summary targets a container, and
`CONTAINER` is a target type of the aggregate with its retention kind, its audit action and its
merge rule recorded the way `JUMBLE_ENTRY`'s were; both are refused for an actor who may not read
what they summarise, with a cross-tenant negative test each; the prompts are new files with versions
and no prompt text goes through the message catalogue; the injection test covers a comment that
issues instructions; no user content reaches a log, metric, trace or audit entry, proved by the PG-4
check; both respect the budget and the breaker and degrade to nothing rather than to an error; both
are in `domain-model.md` §5, registered, and green in the parity test; §2's Summarisation row is
current in both cells.

**Read:** `core/application/service/suggestion/Actions.go` (the `Ask` shape all three share);
`core/domain/model/suggestion/Suggestion.go` (target types, kinds); `domain-model.md` §5;
`ai-first.md` §1.3, §2; `audit.md` §2; `data-retention.md`; `offline-sync.md` §4; ADR-0017

---

## K-06 — `/api/v1/meta/health`: the authenticated door, and the two answers **[L]**

*Depends on: nothing. Closes [#507](https://github.com/Jersyfi/hubtask/issues/507).*

The route the contract declares, `observability-reliability.md` §5 tabulates, `milestone-0.3.5.md`'s
W-03 named in its acceptance, and `presentation/rest/Pending.go` answers `404` to. The report itself
is complete and correct; it is served on the internal listener, with a note beside it saying
"until A-06", and A-06 landed in `0.1.0`.

The issue proposes an override requiring `admin:read`, and that is where the task gets interesting,
because **`admin:read` does not exist**. The installation's admin scope is `admin:tenants`, and
`catalogue.SessionScopes()` withholds every `admin:*` scope from a session on purpose — the admin
surface is entered by a deliberately minted credential, never by whoever happens to be signed in
(`0.6.0` decision 6). The webapp signs in with a session bearer. So the naive fix produces a route
whose one planned client — `apps/webapp/src/lib/data/health.svelte.ts`, built in F1, shipping today,
already polling this path and silently swallowing what comes back — could never call it.

The decision, taken with the milestone: **one route, two answers**, which is the pattern
`GetCapabilities` already uses and states in its own comment — *"the same endpoint, a different
answer, decided by the scope the use case opens rather than by a branch here"*.

* An installation operator, holding `admin:tenants`, reads the whole report: every dependency with
  its latency and its last error code, the circuit states, the backlogs, the configuration warnings,
  the migration state and the version.
* A workspace administrator, holding a scope a session can carry, reads `status` and
  `degraded_features` and nothing else. That is exactly what `HealthNotice.svelte` renders and it
  crosses no tenant boundary: which features are degraded is a statement about what the reader is
  about to try, while the backlogs, the warnings and the dependency names are the installation's
  internals and stay the operator's.

The scope for the second reader is new and is not `admin:*` — the prefix is what `SessionScopes`
filters on, and a name that had to be special-cased there would be a bound that reads one way and
behaves another. It is declared by the descriptor like every other scope, so `Scopes()` grows it
with nothing to remember, and `CreateAccessToken` accepts it for a PAT as well.

Two more things, and both are one sentence in the code rather than a surprise later. The API route
answers `200` even when the status is `down`, because the contract says so in its own description —
*"the HTTP status describes whether the endpoint is reachable, the `status` field describes the
system"*. The ops listener keeps its `503`, because a status page reads a status code and holds no
token. They differ deliberately, and the comment says why.

**Acceptance:** the contract changes first — the description names the scopes that exist rather than
`admin:read`, the reduced answer is in the schema, and `make generate` produces no diff afterwards;
the route is a registered use case with its authorisation in the application layer and nothing in
the adapter deciding who may read what (rule 2); an operator credential receives the full report and
a session receives `status` and `degraded_features` only, proved by a test each, and a caller with
neither scope is refused; the ops listener is unchanged, including its `503`, and its comment no
longer says "until A-06"; `apps/webapp` receives a report where it received a `404`, the
`HealthBanner` renders a degradation end to end, and `packages/sync-engine`'s `HealthReport` type
matches what the route can actually answer to the reader that asks; the stale comment in
`infrastructure/environment/EnvConfig_test.go` about the report being
`route.operation_not_available` is corrected; `observability-reliability.md` §5, `milestone-F1.md`'s
header decision and `docs/evidence/QS-09-2026-09-09.md`'s note about reading the report on the
internal listener are current; no user content and no secret reaches the reduced answer;
`make verify` and `go test -tags contract ./test/contract/...` are green.

**Read:** [#507](https://github.com/Jersyfi/hubtask/issues/507); `observability-reliability.md` §5;
`presentation/rest/OpsController.go`, `MetaController.go`, `Pending.go`;
`core/application/catalogue/Catalogue.go` (`Scopes`, `SessionScopes`);
`core/application/service/sealing/Status.go` (an installation-scoped read as a use case);
`apps/webapp/src/lib/data/health.svelte.ts`; `milestone-F1.md` (the header decision, F1-06);
`api-guidelines.md` §2; ADR-0004; ADR-0005

---

## The order at a glance

```
K-01 (the leak, and the gate that ends it)
K-02 ── K-03   (the same shape; K-02 sets it)
K-04
K-05
K-06 (nothing, and it is the one somebody is waiting for)
```

Nothing here blocks anything else here. K-01 goes first because the test in it is what would have
caught the defect that made this milestone necessary, and K-03 follows K-02 because they are one
pattern applied twice and the second is cheaper once the first has chosen how it looks.

**Definition of Done for the milestone:** every row of `ai-first.md` §2 names either what shipped
and where, or the milestone it moved to, and nothing in the table describes a capability the product
does not have; no prompt file asks a provider for a key the code discards, proved by a test rather
than by having looked; a jumble entry proposes the work under it and the proposal can be accepted;
the classifier proposes a bucket and the values of the fields a workspace declared, both chosen from
sets it was shown and neither of them a destination it invented; near-duplicates are found where
`pgvector` is and nothing is claimed where it is not; a comment thread and a collection can be
summarised, with the comments fenced as context and no user content anywhere near a log; and
`/api/v1/meta/health` answers — the whole report to an operator, the part that concerns them to a
workspace administrator — so that the banner `F1` built has something to render and `0.3.5`'s
acceptance is true.
