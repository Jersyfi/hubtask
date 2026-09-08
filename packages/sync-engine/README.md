# @hubtask/sync-engine

The data seam of the first-party clients: framework-agnostic TypeScript that owns every call to a
Hubtask server, so that no component ever talks to a transport
([ADR-0033](../../docs/adr/ADR-0033-shared-client-architecture.md) §2).

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

The stream and the byte transfer are the same seam:

```ts
const stopStream = engine.listen({
  // What a change record makes stale, as path prefixes. The engine does not know what a hub is.
  pathsFor: (record) => (record.entity === 'work_item' ? [`/items/${record.entity_id}`] : []),
});

await transport.transfer({ url: ticket.url, method: 'PUT', body: file, timeoutMs: 120_000 });
```

`listen` opens one connection per tab, reconnects with the position the stream last sent, and
re-reads what a record names — a record is a signal, never data to apply. The transfer is the one
request that leaves for an address the engine did not compose, and it carries no bearer.

**Online-only today.** The queue, the local store and the hybrid logical clock arrive in F6 with
the protocol they implement; the ports and the subscription API are here now because they are what
everything else is built on.

**It never merges.** That is the server's job, and the rule is a test rather than a paragraph — see
[CLAUDE.md](./CLAUDE.md).
