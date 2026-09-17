// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The TypeScript client's example: list the collections of a hub, create an entry in the first
// one, read it back. Plain JavaScript importing the generated client, so that it runs with
// `node` alone (Node 24 strips the types):
//
//   HUBTASK_URL=https://hubtask.example/api/v1 HUBTASK_TOKEN=hbt_pat_… HUBTASK_HUB=<hub-id> \
//     node sdk/typescript/examples/quickstart.mjs

import { HubtaskClient, ProblemError } from '../src/client.gen.ts';

export async function run({ baseUrl, token, hub, log = console.log }) {
  const client = new HubtaskClient({ baseUrl, token });
  const collections = await client.listContainers({ query: { type: 'COLLECTION', parent_id: hub } });
  if (collections.data.length === 0) throw new Error('the hub has no collection to create in');
  for (const collection of collections.data) log(`${collection.id}  ${collection.name}`);

  // The idempotency key is what makes running this twice after a lost connection safe.
  const first = collections.data[0].id;
  const created = await client.createWorkItem(
    { collection_id: first, type: 'TASK', title: 'Made by the TypeScript SDK' },
    { idempotencyKey: crypto.randomUUID() },
  );
  log(`created ${created.id} (version ${created.version})`);
  const read = await client.getWorkItem(created.id);
  log(`read "${read.title}"`);
  return read;
}

if (process.argv[1] === new URL(import.meta.url).pathname) {
  const { HUBTASK_URL: baseUrl, HUBTASK_TOKEN: token, HUBTASK_HUB: hub } = process.env;
  if (!baseUrl || !token || !hub) {
    console.error('set HUBTASK_URL, HUBTASK_TOKEN and HUBTASK_HUB');
    process.exit(1);
  }
  run({ baseUrl, token, hub }).catch((error) => {
    console.error(error instanceof ProblemError ? `refused: ${error.message}` : error);
    process.exit(1);
  });
}
