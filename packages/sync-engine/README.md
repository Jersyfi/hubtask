# @hubtask/sync-engine

The data seam of the first-party clients: framework-agnostic TypeScript that owns every call to a
Hubtask server, so that no component ever talks to a transport. It implements the client half of
the synchronisation protocol in [`offline-sync.md`](../../docs/architecture/offline-sync.md)
against three ports — `Transport`, `Storage`, `Clock`
([ADR-0033](../../docs/adr/ADR-0033-shared-client-architecture.md) §2). The rules for changing it
are in [`AGENTS.md`](./AGENTS.md).

```ts
import { FetchTransport, SyncEngine } from '@hubtask/sync-engine';

const engine = new SyncEngine({
  transport: new FetchTransport({ baseUrl: '/api/v1' }),
  token: () => platform.bearer(),
});

const stop = engine.subscribe<Account>({ path: '/accounts/me' }, (state) => {
  // 'idle' | 'loading' | 'ready' | 'failed' — one union, so a caller that forgets a state does
  // not compile.
});
```

## Reading

* **A resource** is subscribed to, refreshed and paged. A resource that already holds data keeps
  publishing it while the next answer is on its way: `loading` is published for the first read and
  for a retry after a failure, never over `ready`, so a reload neither collapses a list to its
  skeleton nor takes the keyboard focus with it.
* **`If-Match`.** The engine holds the `ETag` each read carried and hands it to the caller as
  `state.etag`; the caller passes it back through `mutate`'s `ifMatch`. It is not applied behind
  the caller's back, because a write's path and a read's path are only usually the same. A stale
  version is `409 version_conflict`, never `412`
  ([ADR-0025](../../docs/adr/ADR-0025-precondition-failures.md)); `TransportError.isVersionConflict`
  reads the code, because a taken container name is a `409` too.
* **A read that is a `POST`.** `ResourceRequest.body` makes one: `/items:query` and `/search` take
  a document, because a search term in a query string travels through access logs (`security.md`
  §9). Such a resource is keyed on the path *and* the document, with its keys sorted, so two board
  columns are two subscriptions and `{a, b}` and `{b, a}` are one.
* **A page appended.** `loadMore` concatenates rather than replaces. The cursor is the server's —
  opaque, never parsed or built — and goes in the query string of a `GET` and in the `page` of a
  query document. A grouped result is not paged here: each group is continued by asking for that
  group again, which is a different subscription. A page that fails leaves the pages that arrived.
* **Invalidation.** `mutate`'s `invalidates` is a list of path names; omitting it invalidates
  everything. A trailing `$` names the path itself and not what hangs under it (`/items/{id}$` is
  the entry, with or without its query string, and not its comments); `*` stands for one segment.
  `matchesPath` is the one place that reads them, and the webapp keeps its names in
  `apps/webapp/src/lib/data/touches.ts`. A watched entry is read again with the request it was
  loaded with; an unwatched one is forgotten. An invalidation that arrives while the same entry is
  being read shares one follow-up read with everything else that arrives meanwhile, and the
  follow-up is never skipped, because the read in flight may predate the change.

## The replica

`attach(storage, identity)` gives the engine a store — one database per API origin and account,
opened by the platform seam — and mints the device (a UUIDv7 held in the store, §9.1) and the
hybrid logical clock (`hlc.ts`: the server's textual form, physical time from the `Clock`, a
counter, the device; the physical part never moves backwards). `IndexedDbStorage` and
`MemoryStorage` are held to one contract by `test/storage.test.ts`; nothing under `apps/` touches
IndexedDB. There is no encryption in the browser store: what must not sit unencrypted on a shared
machine does not go there at all (ADR-0033 §4), and `clear()` deletes the database rather than
emptying it (§9.6).

`listen` takes the **initial synchronisation** when the store holds no cursor (`POST
/sync:snapshot`, read line by line, its cursor kept from the last line; after a second snapshot
that ends without one, the engine walks `:pull` from nothing) and the **delta** from the held
cursor otherwise — on start, on every reconnect and after every push. The cursor advances in the
store on the frame, so a reload continues where the tab was.

**Applying a record** transcribes a decision the server already took: a whole object replaces the
stored document, a field updates that field, a set record adds or removes an element in the set
held beside the document, a `DELETE` removes the entity and everything under a container, and
`ACCESS_REVOKED` does what a subtree deletion does (§6). An unknown field or entity is stored as it
came (§9.7). Every record is applied and then invalidates what the application's `pathsFor` names
— the engine does not learn what a hub is. `sync.cursor_too_old` empties the store, keeps the
device and synchronises again (§9.4); `sync.cursor_invalid` forgets the cursor and keeps the copy;
`reset()` deletes the store, which is what sign-out means (§9.6).

## Writing

`mutate` writes directly while the queue is empty and the server answers. Otherwise — and when a
direct write fails to reach the server — the application's `mutationFor` — the counterpart of `pathsFor` — maps the write to the mutation
`:push` takes, or answers nothing for a write the frame cannot carry. A queued mutation gets an
`op_id` (UUIDv7, once per intent, §9.2) and a clock per field, is written to the store's `queue` in
order, and its **prediction** is applied to the replica marked `pending`. `push()` sends batches of
at most 500 in order, on every reconnect and at once when something is queued while the server
answers, and applies each result as the server's word: `APPLIED` and `MERGED` write `server_state`
over the prediction, `CONFLICT` writes the server's state and keeps both values in `conflicts`,
`REJECTED` keeps the mutation with its code in `rejected` until dismissed. A push that does not
reach the server leaves the queue as it was; `sync.device_revoked` empties the copy, forgets the
device and keeps what the queue held as refused. `queue()` publishes the count, the oldest moment,
the rejected and the conflicts — what the `SyncStatus` component renders. `ordering.ts` mints the
order keys a reorder needs offline, with the server's cases as its test.

## The network primitives

* **`Transport.stream`** reads `GET /stream` from the response body; `engine.listen` keeps one
  connection per tab. `connectTimeoutMs` bounds the wait for the headers, `idleTimeoutMs` the
  silence between chunks. `401` ends the session through the one hook, the two cursor refusals
  recover as above, and a `503` waits exactly the `Retry-After` the server named.
* **`Transport.transfer`** is the middle step of the three-step upload (arc42 §8.4): the bytes go
  to a presigned URL with no bearer and `credentials: 'omit'`, under a deadline sized by the bytes,
  with progress reported as they leave. `engine.transfer` invalidates only what the caller names.
* **`Transport.document`** is a `POST` whose answer is a file — `POST /views/{id}:export`. The
  response headers come back uninterpreted beside the bytes (`Export-Truncated` is the
  application's to read), and it invalidates nothing.

## Testing

`test/fakes.ts` holds the fake `Transport` and fixed `Clock` every test drives the engine with. The
conformance run plays §9's eight points through the engine's own API against a real instance:

```bash
pnpm --filter @hubtask/sync-engine conformance --base-url … --token …
```

`test/conformance.test.ts` holds it to its claims against `test/fakeServer.ts`, and CI runs it
against the Compose stack (`engine-session`).
