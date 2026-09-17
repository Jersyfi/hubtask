# packages/sync-engine — the client's data seam

Every request a first-party client makes goes through this package, and nothing else in a client
calls `fetch`. It implements the client half of [ADR-0021](../../docs/adr/ADR-0021-offline-sync.md)
against the three ports [ADR-0033](../../docs/adr/ADR-0033-shared-client-architecture.md) §2 names:
`Transport`, `Storage`, `Clock`.

**F1 built the shape; F6-03 built the replica behind it.** `SyncEngine` holds, once a store is
attached, the local copy of the workspace `offline-sync.md` §2 describes: `attach(storage,
identity)` gives it the store — one database per API origin and account, opened by the platform
seam because the engine does not learn what an account is — mints the device (a UUIDv7, §9.1, held
in the store and gone with it) and the hybrid logical clock (`hlc.ts`: the server's textual form,
physical time from the `Clock`, a counter, the device, and `Tick`'s rule that the physical part
never moves backwards). `listen` then takes the **initial synchronisation** where the store holds
no cursor — `Transport.snapshot`, `POST /sync:snapshot`, read line by line into the replica, its
cursor kept from the last line; a snapshot that ends without one is taken again, and after the
second such end the engine walks `:pull` from nothing, page by page — and the **delta** from the
held cursor otherwise, on start, on every reconnect and after every push; and the stream
**applies** every record to the replica before it invalidates what `pathsFor` names. The cursor
advances in the store, not in memory, so a reload continues where the tab was. Nothing under
`apps/` touches IndexedDB: `IndexedDbStorage` and `MemoryStorage` are this package's, and
`test/storage.test.ts` holds both to one contract.

**What applying is, and what it is not.** A record is the server's decision, already taken: a
whole object replaces the stored document, one field updates that field, a set record adds or
removes an element in the entry's set — held beside the document, because the document the server
sends carries no sets — a `DELETE` removes the entity and, for a container, everything under it by
the tree the containers describe, and `ACCESS_REVOKED` does what a subtree deletion does at the root
it names (§6). None of that is a merge: two values for one field and a rule choosing between them
is the server's, and reaches this store as one more record. A field or an entity this build has
never seen is stored as it came (§9.7). The two refusals that concern the store: `sync.cursor_too_old`
empties it — the device stays, it is the copy that is stale — and runs the initial synchronisation
again (§9.4); `sync.cursor_invalid` forgets the cursor and keeps the copy. `reset()` deletes the
store — the copy, the queue-to-be, the device, the cursor — which is what sign-out means (§9.6).

**No encryption in the browser, by decision.** ADR-0033 §4: "no browser-side encryption theatre".
A key the page holds is a key the page's origin can read, and what must not sit unencrypted on a
shared machine does not go into browser storage at all; the promise lives in the shells, whose
keystore is the platform's (ADR-0031). What the browser store promises instead is §9.6's other half,
exactly: `clear()` deletes the database rather than emptying it.

**The queue is F6-05's.** Nothing is queued yet and nothing is applied optimistically: a write
still succeeds or fails in front of the person who made it. `stamp()` and `catchUp()` are the two
things the queue will need from here and already has.

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
  Omitting it invalidates everything, and that is the safe default rather than the lazy one — a
  stale row is worse than a redundant reload. Naming prefixes is what keeps a drag from reloading
  four columns that did not change.

  **What "invalidate" means depends on whether anybody is watching**, and treating the two alike
  was a defect: an entry with listeners is a screen somebody has open, and *dropping* it takes the
  listeners with it — so the component that made the write is never told and the change it just
  performed does not appear. Watched entries are therefore read again, with the request they were
  loaded with so a query keeps its document; unwatched ones are forgotten, because reloading a
  cache nobody is looking at is a burst of requests for nothing.

**F5-11 gave it one rule about what a screen sees during a reload.** A resource that already
holds data keeps publishing it while the next answer is on its way; `loading` is published for
the first read and for a retry after a failure, never over a `ready` state. Before that, every
write tore the list that made it down to its skeleton for the length of the reload — and took the
keyboard's focus to `body` with it, so a reorder by menu cost the reader the whole tab order back
to the row. A reload is invisible until it lands, and lands as `ready` like the first.

**F3-04 gave it the two network primitives it did not have.** Both belong here because both are
`fetch`, and there is one caller of `fetch`:

* **A response that does not end.** `Transport.stream` reads `GET /stream` from the response body,
  and `engine.listen` keeps one connection open per tab. It is **never `EventSource`**: that API
  cannot carry a header, so authenticating it would mean a token in the URL, which `security.md`
  forbids and which access logs and browser history would keep — `test/rules.test.ts` fails on the
  name. A stream is the one call whose deadline is not a deadline: `connectTimeoutMs` bounds the
  wait for the headers, and `idleTimeoutMs` bounds the silence between chunks, because the body is
  meant never to end.

  A record is **the server's decision, transcribed to the replica, and a signal to re-read**.
  Nothing in it is merged with anything — the engine writes what the record says and re-reads
  what it names. Which prefixes comes from `pathsFor`, which the application supplies — the engine
  does not learn what a hub is. The four
  refusals are four recoveries and the engine tells them apart: `401` ends the session through the
  one hook, `sync.cursor_too_old` drops everything held — the store included, since F6-03 — and
  restarts with no cursor (a delta across a gap would be silently wrong, `offline-sync.md` §7),
  `sync.cursor_invalid` restarts with no cursor and drops nothing, and a `503` waits exactly the
  `Retry-After` the server named. The cursor advances on the frame rather than on the record, so a
  reconnect never asks for a record it already has; with a store attached it advances there, and a
  reload continues where the tab was. Since F6-03 a record is applied to the replica *and* is a
  signal to re-read — the re-read is what a screen shows while the copy is what it reads offline.

* **A body that is bytes.** `Transport.transfer` is the middle step of the three-step upload
  (arc42 §8.4) and the **one** request that may leave for an address the engine did not compose.
  It carries **no bearer** and `credentials: 'omit'`: a presigned URL is its own credential, and a
  bearer sent to a bucket is a bearer leaked to a third party. Its deadline is sized by the bytes
  rather than by the API's, and progress is reported as they leave. Staging and confirming are
  ordinary `mutate` calls, not a second write path. `engine.transfer` is a pass-through so that
  an application holds one seam rather than two, and it invalidates **only** what a caller
  names — unlike `mutate`, whose omitted `invalidates` means everything, because bytes in a
  bucket change nothing the client is holding until the confirmation says so.

**F3-15 gave it a third.** `Transport.document` is a `POST` whose answer is a **file rather than
data** — `POST /views/{id}:export`, which renders CSV, JSON or iCalendar. It is a `POST` for the
reason `/search` is, so it cannot be a navigation, and a navigation is what a browser downloads by;
hence a request that carries a bearer like every other and hands back bytes. The response headers
come back with them because one of them is the answer — `Export-Truncated` says the file is the
first page of a larger result — and they are handed over uninterpreted: what that header *means* is
the application's business. Like `transfer`, it invalidates nothing, because an export is a read
whatever its verb.

## What must not happen here

* **No merging. Ever.** Merging is the server's (ADR-0021, `offline-sync.md` §4). The engine
  queues, pushes, applies what the server answers, and surfaces the conflict for the UI to render.
  A merge rule in this package is a bug against that decision rather than a feature, and
  `test/rules.test.ts` fails on a symbol that merges. Concatenating page two onto page one is not
  one: nothing is reconciled, and the order is the server's.
* **No optimistic apply, and therefore no rollback.** A write still either succeeds or fails in
  front of the person who made it. Rolling one back would be a guess about a `:push` the queue does
  not send yet (F6-05), and the queue arrives with the protocol that decides what a rollback is.
  Applying a *server's* record to the replica is not that: it is transcription of a decision the
  server has taken.
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
