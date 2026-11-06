// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// One test for every Storage (ADR-0033 §2): the memory store, the IndexedDB store and the shells'
// SQLite store are held to the same promises, in the same file. The IndexedDB store runs here over
// a fake of the API slice it uses (test/fakeIndexedDb.ts), and the real engines open it in the
// browser job; the SQLite store runs over a fake of the shell's commands (test/fakeShell.ts), and
// the shell's own tests hold the encrypted database behind them to the same cases (copy.rs).

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { Storage } from '../src/ports.ts';
import { MemoryStorage } from '../src/storage/MemoryStorage.ts';
import { IndexedDbStorage, databaseNameFor } from '../src/storage/IndexedDbStorage.ts';
import { SqliteStorage, copyNameFor } from '../src/storage/SqliteStorage.ts';
import { fakeIndexedDb } from './fakeIndexedDb.ts';
import { FakeShell } from './fakeShell.ts';

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
  {
    name: 'SqliteStorage',
    fresh: () => {
      const shell = new FakeShell();
      const name = copyNameFor('https://hubtask.example', 'account-1');
      return {
        storage: new SqliteStorage(name, shell.call),
        reopen: () => new SqliteStorage(name, shell.call),
        exists: () => shell.files.has(name),
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

  // A file captured offline waits in the store as bytes (offline-sync.md §1) and is uploaded from
  // what comes back, so each kind the engine accepts returns as itself.
  test(`${name}: bytes come back as the kind they went in as`, async () => {
    const { storage } = fresh();
    const record = {
      array: new Uint8Array([0, 1, 254, 255]),
      buffer: new Uint8Array([9, 8]).buffer,
      blob: new Blob([new Uint8Array([7, 7, 7])], { type: 'application/pdf' }),
      nested: [{ bytes: new Uint8Array([42]) }],
    };
    await storage.put('files', 'op-1', record);
    const back = await storage.get<typeof record>('files', 'op-1');
    assert.ok(back?.array instanceof Uint8Array);
    assert.deepEqual([...back.array], [0, 1, 254, 255]);
    assert.ok(back.buffer instanceof ArrayBuffer);
    assert.deepEqual([...new Uint8Array(back.buffer)], [9, 8]);
    assert.ok(back.blob instanceof Blob);
    assert.equal(back.blob.type, 'application/pdf');
    assert.deepEqual([...new Uint8Array(await back.blob.arrayBuffer())], [7, 7, 7]);
    assert.deepEqual([...(back.nested[0]?.bytes ?? [])], [42]);
    const listed = await storage.all<typeof record>('files');
    assert.deepEqual([...(listed[0]?.array ?? [])], [0, 1, 254, 255]);
  });

  test(`${name}: clear leaves nothing, and the store works again afterwards (§9.6)`, async () => {
    const { storage, reopen, exists } = fresh();
    await storage.put('items', 'a', { id: 'a' });
    await storage.put('meta', 'device', { id: 'd' });
    await storage.clear();
    assert.equal(exists() && name !== 'MemoryStorage', false, 'the database is deleted, not emptied');
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

// The window a reused transaction meets: the writing transaction is kept for the next request, and between its
// last request's completion and its `complete` event it is finished but still held. A request
// issued on it from a later task throws `TransactionInactiveError`; the store answers from a
// fresh transaction rather than reporting the copy empty.
test('IndexedDbStorage: a read or write after the writing transaction finished is answered, not thrown', async () => {
  const { factory } = fakeIndexedDb();
  const storage = new IndexedDbStorage(databaseNameFor('https://hubtask.example', 'account-2'), factory);
  await storage.put('items', 'i1', { title: 'first' });

  // A task later: the transaction has gone inactive, and its `complete` has not yet fired.
  await new Promise<void>((resolve) => setTimeout(resolve, 0));
  assert.deepEqual(await storage.get('items', 'i1'), { title: 'first' });

  await new Promise<void>((resolve) => setTimeout(resolve, 0));
  await storage.put('items', 'i2', { title: 'second' });
  await new Promise<void>((resolve) => setTimeout(resolve, 0));
  assert.deepEqual(await storage.all('items'), [{ title: 'first' }, { title: 'second' }]);
  await new Promise<void>((resolve) => setTimeout(resolve, 0));
  await storage.delete('items', 'i1');
  assert.deepEqual(await storage.all('items'), [{ title: 'second' }]);
});

test('SqliteStorage: the copy is named by origin and account, escaped so no two share one and no path is read out of it', () => {
  assert.equal(copyNameFor('https://hubtask.example', 'acc-1'), 'https_3A_2F_2Fhubtask.example/acc-1');
  assert.equal(copyNameFor('http://localhost:8080', 'acc-1'), 'http_3A_2F_2Flocalhost_3A8080/acc-1');
  assert.notEqual(copyNameFor('https://hubtask.example', 'acc-1'), copyNameFor('https://hubtask.example', 'acc-2'));
  assert.notEqual(copyNameFor('https://a.example', 'acc-1'), copyNameFor('https://b.example', 'acc-1'));
  // An underscore of the origin's own is escaped too, so the escaping is reversible.
  assert.notEqual(copyNameFor('http://a_3A', 'x'), copyNameFor('http://a:', 'x'));
  assert.match(copyNameFor('https://ü.example/../x', '../y'), /^[A-Za-z0-9._-]+\/[A-Za-z0-9._-]+$/);
  assert.equal(copyNameFor('https://hubtask.example', '../y').split('/')[1], '.._2Fy');
});

test('SqliteStorage: clear deletes the copy and keeps its key; forget - sign-out - removes both', async () => {
  const shell = new FakeShell();
  const name = copyNameFor('https://hubtask.example', 'account-1');
  const storage = new SqliteStorage(name, shell.call);
  await storage.put('items', 'a', { id: 'a' });
  assert.ok(shell.files.has(name) && shell.keys.has(name));

  await storage.clear();
  assert.equal(shell.files.has(name), false);
  assert.equal(shell.keys.has(name), true, 'a reset keeps the account\'s key');

  await storage.put('items', 'b', { id: 'b' });
  await storage.forget();
  assert.equal(shell.files.has(name), false, 'the file is gone');
  assert.equal(shell.keys.has(name), false, 'and the key');
});
