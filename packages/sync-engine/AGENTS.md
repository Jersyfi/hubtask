# packages/sync-engine — the client's data seam

Every request a first-party client makes goes through this package: reads, writes, the change
stream, the replica and the offline queue. How it works is in [`README.md`](./README.md); the
protocol it implements is [`offline-sync.md`](../../docs/architecture/offline-sync.md).

## What must not happen here

* **No second caller of `fetch`** — in this package or in any first-party client. `FetchTransport`
  is the only one, which makes three promises checkable in one file: every request carries its
  bearer, its `Idempotency-Key` where the operation takes one, and a deadline (rule 7). There is no
  default of "forever". The webapp's session exchange goes through the transport too. The one
  request that leaves for an address the engine did not compose is `Transport.transfer` to a
  presigned URL, and it carries no bearer and `credentials: 'omit'`.
  `[partial: ci:node; open: a fetch call in another first-party client, globalThis.fetch]`
* **No `EventSource`.** It cannot carry a header, so authenticating it would put a token in the
  URL, which `security.md` forbids. The stream is read from a `fetch` response body.
  `[gate: ci:node]`
* **No merging. Ever.** Merging is the server's (`offline-sync.md` §4). The engine queues, pushes,
  applies what the server answers and surfaces the conflict for the UI. A prediction is what the
  replica would hold if the server applied the mutation as sent, and the server's answer overwrites
  it whole (§9.5). Applying a server record or answer to the replica is transcription, not a
  merge, and so is appending page two to page one.
  `[partial: ci:node; open: merge logic under another name]`
* **No rollback.** A refused mutation's prediction is replaced by the server's state where the
  server named one, the rejection is kept and shown until dismissed, and the copy is otherwise read
  again. A rollback would be this side deciding what the entry says.
  `[partial: ci:node; open: paths the tests do not drive]`
* **No write both queued and sent, and none the frame cannot carry queued.** A first-party client
  writes directly while its queue is empty and the server answers. Otherwise a write the push frame
  carries is queued with its `op_id` and clocks and shown pending; one it cannot carry is refused
  with `sync.needs_connection` where the person made it. Never both, never rolled back
  (`offline-sync.md` §1). `[partial: ci:node; open: paths the tests do not drive]`
* **No product knowledge.** The engine does not learn what a hub, an entry or an account is. Which
  paths a record makes stale comes from the application's `pathsFor`; which write is which mutation
  from its `mutationFor`; which store belongs to which account from the platform seam. `[owner]`
* **No read that invalidates, and no write that invalidates more than it names.** A read that is a
  `POST` (`/items:query`, `/search`, an export) is keyed on its path and its document and
  invalidates nothing. A write invalidates the paths it names and nothing else; omitting
  `invalidates` means everything, which is the safe default. `engine.transfer` invalidates only
  what its caller names. `[partial: ci:node; open: paths the tests do not drive]`
* **No `loading` over data a screen already shows.** A reload keeps publishing what it holds until
  the answer lands. `[partial: ci:node; open: paths the tests do not drive]`
* **No token held.** The bearer is asked for per call through the function the platform seam
  supplies; a copy taken at construction keeps working after a sign-out.
  `[partial: ci:node; open: a copy taken on a path the tests do not drive]`
* **No encryption in the browser store.** What must not sit unencrypted on a shared machine does
  not go into browser storage at all (ADR-0033 §4). `clear()` deletes the database rather than
  emptying it, and nothing under `apps/` touches IndexedDB.
  `[partial: ci:node; open: encryption added, IndexedDB used under apps/]`
* **No framework.** No Svelte, no React, nothing that needs a DOM: the engine must run headless.
  The Svelte binding lives in `apps/webapp/src/lib/data/`.
  `[partial: ci:node; open: DOM globals used without a framework import]`
* **No display text.** A failure carries the server's message code and its parameters (rule 8).
  `[partial: ci:node; open: prose in a thrown error]`
* **No dependency but `@hubtask/api-client`**, and none on `apps/*`.
  `[partial: ci:node; open: a peer dependency]`

`test/rules.test.ts` fails, for this package, on a symbol that merges, a framework, `EventSource`
and a call of `fetch` outside `FetchTransport.ts`. The merge and `EventSource` checks skip comment
lines; the `fetch` check reads comments too, so do not write the call with its parenthesis in one.

## How to check a change

```bash
pnpm --filter @hubtask/sync-engine test       # headless, against the fakes in test/fakes.ts
pnpm --filter @hubtask/sync-engine typecheck
pnpm --filter @hubtask/sync-engine lint
node build/lint-workspace-map.mjs             # from the repository root: the workspace edges
```

A fake `Transport` or `Clock` belongs in `test/fakes.ts`, not inline in one test file: the same
fakes drive the conformance harness. A change to the protocol behaviour is held by
`test/conformance.test.ts` against `test/fakeServer.ts`; CI also runs the conformance run against
the Compose stack.
