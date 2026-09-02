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

**Online-only today.** The queue, the local store and the hybrid logical clock arrive in F6 with
the protocol they implement; the ports and the subscription API are here now because they are what
everything else is built on.

**It never merges.** That is the server's job, and the rule is a test rather than a paragraph — see
[CLAUDE.md](./CLAUDE.md).
