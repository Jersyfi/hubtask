// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// One test for every Storage (ADR-0033 §2): the memory store and the IndexedDB store are held to
// the same promises, in the same file, so that a third implementation - the shells' SQLite - has
// the contract written down before it is written. The IndexedDB store runs here over a fake of
// the API slice it uses (test/fakeIndexedDb.ts); the real engines open it in the browser job.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { Storage } from '../src/ports.ts';
import { MemoryStorage } from '../src/storage/MemoryStorage.ts';
import { IndexedDbStorage, databaseNameFor } from '../src/storage/IndexedDbStorage.ts';
import { fakeIndexedDb } from './fakeIndexedDb.ts';

interface Implementation {
  readonly name: string;
  readonly fresh: () => { storage: Storage; reopen: () => Storage; exists: () => boolean };
}

const IMPLEMENTATIONS: readonly Implementation[] = [
  {
    name: 'MemoryStorage',
    fresh: () => {
      const storage = new MemoryStorage();
      return { storage, reopen: () => storage, exists: () => true };
    },
  },
  {
    name: 'IndexedDbStorage',
    fresh: () => {
      const { factory, fake } = fakeIndexedDb();
      const name = databaseNameFor('https://hubtask.example', 'account-1');
      return {
        storage: new IndexedDbStorage(name, factory),
        reopen: () => new IndexedDbStorage(name, factory),
        exists: () => fake.databases.has(name),
      };
    },
  },
];

for (const { name, fresh } of IMPLEMENTATIONS) {
  test(`${name}: a record is put, read back, listed and deleted`, async () => {
    const { storage } = fresh();
    assert.equal(await storage.get('items', 'a'), undefined);

    await storage.put('items', 'a', { id: 'a', title: 'one' });
    await storage.put('items', 'b', { id: 'b', title: 'two' });
    await storage.put('containers', 'c', { id: 'c' });
    assert.deepEqual(await storage.get('items', 'a'), { id: 'a', title: 'one' });
    assert.deepEqual(
      ((await storage.all<{ id: string }>('items')).map((r) => r.id)).sort(),
      ['a', 'b'],
    );
    assert.deepEqual([...(await storage.collections())].sort(), ['containers', 'items']);

    await storage.put('items', 'a', { id: 'a', title: 'one, again' });
    assert.deepEqual(await storage.get('items', 'a'), { id: 'a', title: 'one, again' }, 'a put replaces');

    await storage.delete('items', 'a');
    assert.equal(await storage.get('items', 'a'), undefined);
    assert.deepEqual((await storage.all<{ id: string }>('items')).map((r) => r.id), ['b']);
    await storage.delete('items', 'never-there');
  });

  test(`${name}: a collection this build never named is a collection like any other (§9.7)`, async () => {
    const { storage } = fresh();
    const record = { id: 'r', document: { id: 'r', kind: 'something_newer', extra: { nested: [1, 2] } } };
    await storage.put('something_newer', 'r', record);
    assert.deepEqual(await storage.get('something_newer', 'r'), record);
    assert.ok((await storage.collections()).includes('something_newer'));
  });

  test(`${name}: what is held is a copy, not the caller's object`, async () => {
    const { storage } = fresh();
    const given = { id: 'a', tags: ['x'] };
    await storage.put('items', 'a', given);
    given.tags.push('y');
    assert.deepEqual(await storage.get('items', 'a'), { id: 'a', tags: ['x'] });
    const read = await storage.get<{ tags: string[] }>('items', 'a');
    read?.tags.push('z');
    assert.deepEqual(await storage.get('items', 'a'), { id: 'a', tags: ['x'] });
  });

  test(`${name}: clear leaves nothing, and the store works again afterwards (§9.6)`, async () => {
    const { storage, reopen, exists } = fresh();
    await storage.put('items', 'a', { id: 'a' });
    await storage.put('meta', 'device', { id: 'd' });
    await storage.clear();
    assert.equal(exists() && name === 'IndexedDbStorage', false, 'the database is deleted, not emptied');
    assert.equal(await storage.get('items', 'a'), undefined);
    assert.equal(await storage.get('meta', 'device'), undefined);
    assert.deepEqual(await storage.collections(), []);
    // And a store opened again under the same name starts empty and takes writes.
    const again = reopen();
    assert.equal(await again.get('items', 'a'), undefined);
    await again.put('items', 'b', { id: 'b' });
    assert.deepEqual(await again.get('items', 'b'), { id: 'b' });
  });
}

test('IndexedDbStorage: the database name is the origin and the account, so two accounts never share one', () => {
  assert.equal(databaseNameFor('https://hubtask.example', 'acc-1'), 'hubtask:https://hubtask.example:acc-1');
  assert.notEqual(databaseNameFor('https://hubtask.example', 'acc-1'), databaseNameFor('https://hubtask.example', 'acc-2'));
  assert.notEqual(databaseNameFor('https://a.example', 'acc-1'), databaseNameFor('https://b.example', 'acc-1'));
});

test('IndexedDbStorage: a runtime with no IndexedDB is refused at construction, not at first use', () => {
  assert.throws(() => new IndexedDbStorage('x', undefined), TypeError);
});
