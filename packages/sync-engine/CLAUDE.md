# packages/sync-engine — the client's data seam

Every request a first-party client makes goes through this package, and nothing else in a client
calls `fetch`. It implements the client half of [ADR-0021](../../docs/adr/ADR-0021-offline-sync.md)
against the three ports [ADR-0033](../../docs/adr/ADR-0033-shared-client-architecture.md) §2 names:
`Transport`, `Storage`, `Clock`.

**F1 built the shape and not the behaviour.** The engine is online-only: no queue, no local store,
no hybrid logical clock. Those implement `:pull` and `:push`, which have no server yet, and a
client written against a protocol that does not exist is a client written twice — they arrive in
F6 with `offline-sync.md` §9. What is here now is what everything else is built on, so that when
the queue lands behind `SyncEngine`, no component changes.

**F2-03 gave it four things a screen needs and F1 did not.** They are worth knowing before adding
a fifth:

* **`If-Match`.** The contract declares it on every `PATCH`, on `DELETE` and on twenty other
  operations, and the engine holds the `ETag` each read carried. It is handed to the caller as
  `state.etag` and passed back explicitly through `mutate`'s `ifMatch` — **not** applied behind a
  caller's back, because a write goes to a path and a read came from a path and the two are only
  usually the same one. A stale version is `409 version_conflict` and never `412`
  ([ADR-0025](../../docs/adr/ADR-0025-precondition-failures.md)); `TransportError.isVersionConflict`
  reads the code rather than the status, because a taken container name is also a `409`.
* **A read that is a `POST`.** `ResourceRequest.body` makes it one. `/items:query` and `/search`
  read — a query is a document, and a search term in a query string travels through access logs
  (`security.md` §9, ADR-0018) — so they are subscribed to, refreshed and paged like anything else
  and they invalidate **nothing**, because they wrote nothing. Such a resource is keyed on the path
  *and* the document, with the document's keys sorted: two columns of one board are both
  `POST /items:query`, and `{a, b}` and `{b, a}` are one question.
* **A page appended.** `loadMore` concatenates rather than replaces, because `LoadMore` is a
  control a person presses and what they had must still be there afterwards. The cursor is the
  server's — opaque and signed, never parsed, never constructed — and goes in the query string of a
  `GET` and in the `page` of a query document. A **grouped** result is not paged here: each group
  is continued by asking for that group again, which is a different question and therefore a
  different subscription. A page that fails throws and leaves the pages that arrived in place.
* **Invalidation that names what changed.** `mutate`'s `invalidates` is a list of path prefixes.
  Omitting it drops everything, and that is the safe default rather than the lazy one — a stale row
  is worse than a redundant reload. Naming prefixes is what keeps a drag from reloading four
  columns that did not change.

## What must not happen here

* **No merging. Ever.** Merging is the server's (ADR-0021, `offline-sync.md` §4). The engine
  queues, pushes, applies what the server answers, and surfaces the conflict for the UI to render.
  A merge rule in this package is a bug against that decision rather than a feature, and
  `test/rules.test.ts` fails on a symbol that merges. Concatenating page two onto page one is not
  one: nothing is reconciled, and the order is the server's.
* **No optimistic apply, and therefore no rollback.** A write still either succeeds or fails in
  front of the person who made it. Rolling one back would be a guess about a `:push` that does not
  exist, and the queue arrives in F6 with the protocol that decides what a rollback is.
* **No framework.** No Svelte, no React, nothing that needs a DOM. The engine has to be
  exercisable headlessly — it is the first-party counterpart to `hubctl sync-conformance` — and a
  package that imported a framework could not be. The Svelte binding lives in
  `apps/webapp/src/lib/data/`, and it is twenty lines because everything else is here.
* **No second caller of `fetch`.** `FetchTransport` is the only one, which is what makes three
  promises checkable in one file instead of reviewed in fifty: every request carries its bearer,
  its `Idempotency-Key` where the operation takes one, and **a deadline** — a client call without
  one is the same defect as a server one, and there is deliberately no default of "forever".
* **No token held.** The bearer is asked for per call through a function the platform seam
  supplies. A copy taken at construction is a copy that keeps working after a sign-out.
* **No display text.** A failure carries the server's message code and its params, never a
  sentence (ADR-0011). The renderer resolves it (F1-07).
* **No dependency but `@hubtask/api-client`.** That is the one workspace edge ADR-0033 §3
  sanctions. `packages/* → apps/*` stays forbidden, and `build/lint-workspace-map.mjs` enforces
  both.

## How to check a change

```bash
pnpm --filter @hubtask/sync-engine test       # headless, against the fakes in test/fakes.ts
pnpm --filter @hubtask/sync-engine typecheck
node build/lint-workspace-map.mjs             # from the repository root: the edges of ADR-0033
```

`test/fakes.ts` is exported from the test directory rather than inlined in one file on purpose: the
same fake `Transport` and fixed `Clock` will drive the conformance run when F6 brings the protocol,
and a fake only one file can reach is a fake that gets rewritten.
