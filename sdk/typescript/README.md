# The TypeScript SDK

`sdk/typescript` is the TypeScript client for the Hubtask API, generated from
[`api/openapi.yaml`](../../api/openapi.yaml) by `make generate` (through `tools/sdkgen`, P-03).
One class over `fetch`, one method per operation, typed against the `operations` the contract
declares — and no runtime dependency: a library that pulls a tree into somebody's project is a
library they audit before they adopt.

```ts
import { HubtaskClient, ProblemError } from '@hubtask/sdk-typescript';

const client = new HubtaskClient({ baseUrl: 'https://hubtask.example/api/v1', token });
const collections = await client.listContainers({ query: { type: 'COLLECTION', parent_id: hub } });
try {
  const created = await client.createWorkItem(
    { collection_id: collections.data[0].id, type: 'TASK', title: 'x' },
    { idempotencyKey: crypto.randomUUID() },
  );
} catch (refused) {
  if (refused instanceof ProblemError) console.log(refused.status, refused.problem.code);
}
```

[`examples/quickstart.mjs`](./examples/quickstart.mjs) lists a hub's collections, creates an
entry and reads it back; `node sdk/typescript/examples/quickstart.mjs` with `HUBTASK_URL`,
`HUBTASK_TOKEN` and `HUBTASK_HUB` set (Node 24 strips the types).

Two generated files, nothing hand-written:

* `src/client.gen.ts` — the client, written by `tools/sdkgen` on `make generate` and committed,
  because the generator is Go and the Node lanes have none.
* `dist/schema.d.ts` — the `operations` types the client imports, written by `pnpm build` with
  `openapi-typescript` from the same document, and ignored by git. This is the SDK's **own**
  generation of the types, not an import of `@hubtask/api-client`'s: the SDK is Apache-2.0 and
  the first-party types are not, so nothing first-party sits in its import path
  ([ADR-0059](../../docs/adr/ADR-0059-licensing-phases-and-licensing-start.md) §6). It is the
  "regeneration rather than a copy" ADR-0057 foresaw for an extraction, done in place.

The package is a workspace member so that the Node lane builds, typechecks and tests it, and an
island on the workspace map ([`project-structure.md`](../../docs/architecture/project-structure.md)
§2.1): it depends on no other member, and no other member depends on it — the first-party apps'
fetch layer is the sync engine's port, never this class. `scripts/client.test.js` drives the client
against a recording server; `pnpm lint` refuses anything in `src/` but the generated file.

**Licence.** Apache-2.0 — the `LICENSE` file beside this README, and the header every file
carries (ADR-0059 §6, deciding [ADR-0057](../../docs/adr/ADR-0057-sdk-licence-and-extraction.md)).
The npm name and an extraction into a repository of its own stay open; the workspace name
`@hubtask/sdk-typescript` is not a publication name.
