# ADR-0054 — An embedding narrower than the index is padded, and a wider one is refused

**Status:** proposed · **Date:** 2026-09-11

## Context

`item_embedding.embedding` is `vector(1536)`. Migration 0075 fixed the width with its reasoning
beside it: an HNSW index is built for one geometry, a table holding two would rank the mixture by
nothing, and 1536 is what the OpenAI-compatible embedding models in common use produce. The
comment goes on: *"what an installation configuring a model of another width has to re-index
for"* — which names a re-index nothing offers, for a width nothing checks.

Nothing between the provider and the column reads the width. `EmbeddingResult.Dimensions` was
carried by [ADR-0049](./ADR-0049-ai-provider-surface.md) decision 4 *"so a caller can reject a batch
that does not match its index"*, and no caller does; `ProviderCapabilities.EmbeddingDimensions`
is declared and neither populated nor read; `EmbeddingRepository.Store` binds the literal straight
into `$3::vector`. So the mismatch is discovered by PostgreSQL, as a raw error on every embedding
job, for as long as the model stays configured.

That is not a corner. The two Ollama embedding models in common use are 768 (`nomic-embed-text`)
and 1024 (`mxbai-embed-large`) dimensions wide — measured, not read off a card — and
[`ai-first.md`](../architecture/ai-first.md) §3 says *"local models become the norm in
self-hosting… the Ollama adapter from day one"*. An operator who takes that sentence at its word
has a search that is lexical for a reason nobody tells them and a job that fails until they stop it.
Semantic search and duplicate detection were, in effect, OpenAI-only, and no document said so.
[#551](https://github.com/Jersyfi/hubtask/issues/551) records the finding; #532, the measurement of
the duplicate threshold, is blocked on it, because two of the three models it names cannot be stored.

Three constraints shape the answer:

* **Rule 12**: migrations are forward-only and safe for rolling updates. A column whose width
  depends on the installation's configuration is a migration that cannot be written once.
* **The index is built for one geometry** — 0075's own reasoning, and it stands. pgvector's HNSW
  index needs a typed dimension; an untyped `vector` column can be indexed only through a cast
  expression per width, which every query would then have to repeat to hit the index.
* **The only distance the product uses is cosine.** `1 - (a <=> b)` in
  `EmbeddingRepository.go` and `query/Compiler.go`, and nowhere `<->` or `<#>`.

## Decision

**1. The index stays 1536 wide, and a narrower vector is zero-padded to it before it is written or
compared.** Cosine similarity is exactly preserved: appending zeros to both vectors changes neither
the dot product nor either norm, so every distance the index is built on and every similarity the
floor is compared against is the number the model produced. Padding happens in one place —
`vectorLiteral`, which both the store and the search's query vector go through — so the two sides
cannot disagree.

**2. A wider vector is refused, in the application layer, before anything reaches the database.**
The job reads `EmbeddingResult.Dimensions` — the field carried for exactly this — and answers
`ai.embedding_too_wide` with the model, its width and the index's. It *finishes* rather than
retrying, because a model's width does not change on the next attempt; the search falls back to
lexical, the way it already does for every other way of not getting an answer (J-10). The adapter
refuses the same vector a second time, with the same code, so a caller that bypassed the check
could not reach the column either.

**3. Padding adds nothing to compare across a model's line — and where that line is drawn is the
same place it was drawn before.** `Near` compares only rows of the entry's own model
(`e.model = me.model`), and a reconfiguration re-embeds everything under the new name (`Owed` calls
the old rows stale). The hybrid search does **not** filter by model: between a reconfiguration and
the pass that finishes re-embedding, it ranks a query against whatever rows exist, of whatever
model. That window is ADR-0049 decision 4's concern and exists for a switch between two 1536-wide
models exactly as it does for a switch to a 768-wide one; this decision neither opens it nor closes
it, and it is recorded here so the padding is not mistaken for a guard it is not.

**4. The width is the repository port's constant, and a test holds the column to it.** One number,
declared where the storage contract is, and an integration test that reads `atttypmod` off the
migrated column — because a constant that mirrors a migration is a number kept twice, and the test
is what makes the second copy honest.

**5. The documents say which widths the product takes.** `ai-first.md` §2's semantic search row
names the bound and the two behaviours, and `deployment.md`'s model guidance says what an operator
choosing a model has to know.

## Options

**Widen the column to the widest model in use.** 3072 for `text-embedding-3-large`, 4096 for the
7B instruct models. Doubles or nearly triples the storage of every installation for the benefit of
the few that would configure one, and the next model is wider still. Not taken; a wider index is a
future migration for a demonstrated need, expand/contract as rule 12 asks. (The OpenAI models that
exceed 1536 accept a `dimensions` request parameter that would bring them under it; **neither
adapter sends one**, so until somebody adds that knob a model above 1536 is unusable, and the
documents say so rather than pointing at a setting that does not exist.)

**An untyped column and a cast expression index per width.** `CREATE INDEX … ((embedding::vector(768)))`
for each width an installation uses. The migration cannot know the widths, so the indexes would be
created at runtime by the application — DDL on the write path, and every query rewritten to repeat
the cast or miss the index. Not taken.

**Refuse everything but 1536, gracefully.** Closes the raw error and leaves the Ollama sentence in
§3 a lie. Not taken on its own; it is decision 2 for the wide case, and the narrow case is the
common one.

**Truncate a wider vector to 1536.** Exact only for models trained for it (Matryoshka
representation learning), and silently wrong for every other — a similarity that looks like one
and measures something else. Not taken; refused instead.

## Consequences

* A 768-wide vector costs 6 KB in the column rather than 3 KB. Ten thousand entries are 30 MB
  rather than 15; the trade is accepted and written here so nobody rediscovers it as waste.
* A model wider than 1536 is a documented refusal rather than a raw error. The search stays
  lexical and the worker logs why, with the job and the workspace named — a repeating job records
  no outcome of its own, so the log line is the only place it is said.
* **The refusal is paid for.** The width is known only once a batch has been embedded, so a wide
  model costs one batch of embedding per pass, every interval, and one query embedding per search,
  each discarded, and both drawn from the workspace's AI budget. For a local model that is nothing;
  for a metered one it is a slow drain. Two things would end it and both are deferred: populating
  `ProviderCapabilities.EmbeddingDimensions` — from configuration for an OpenAI-compatible provider,
  from `/api/show` for Ollama — so the job refuses before it calls; and remembering the refusal
  somewhere the search and `/meta/capabilities` can read it. Neither is in this decision, and
  their absence is the cost of it rather than a property to defend.
* **No degradation signal, yet.** `/meta/capabilities` keeps answering `semantic_search: true` and
  `/meta/health` sees only the breaker, where ADR-0050's two other degradations both surface. The
  honest reason is the one above: a refusal that nothing remembers is a refusal nothing can report.
  It belongs with the deferred work, not with a claim that the current behaviour is right.
* #532 becomes measurable: the two local models are models the product stores, so where their
  paraphrase bands sit is a question about the product rather than about a fixture.
* `ProviderCapabilities.EmbeddingDimensions` stays declared and unread. Populating it would let an
  OpenAI-compatible configuration be refused before the first call, and it is a smaller task than
  this one; it is left for the moment somebody wants it rather than done here for completeness.
