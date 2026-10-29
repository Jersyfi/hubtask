// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import {
  LIST_KEY,
  addressOf,
  entryList,
  handoverFragment,
  hopFragment,
  readEntry,
  readHandover,
  readHop,
  type ListStorage,
} from './workspaces.ts';

function fakeStorage(): ListStorage & { values: Record<string, string> } {
  const values: Record<string, string> = {};
  return {
    values,
    getItem: (key) => values[key] ?? null,
    setItem: (key, value) => {
      values[key] = value;
    },
    removeItem: (key) => {
      delete values[key];
    },
  };
}

/** An entry as the installation signs one: the payload, a dot, a tag this side never checks. */
function signed(payload: object, tag = 'c2lnbmF0dXJl'): string {
  return `${Buffer.from(JSON.stringify(payload)).toString('base64url')}.${tag}`;
}

const acme = signed({ host: 'acme.hub.example', name: 'Acme Ltd' });
const meier = signed({ host: 'meier.hub.example', name: 'Meier family' });

test('the stored entry holds the host and the name and nothing else (UC-ID-25 check 1)', () => {
  const storage = fakeStorage();
  const list = entryList(storage);
  list.keep(acme);
  // An entry carrying anything more - an address, a token - is not kept.
  list.keep(signed({ host: 'leaky.hub.example', name: 'Leaky', email: 'lena@example.com' }));
  list.keep(signed({ host: 'token.hub.example', name: 'Token', token: 'hbt_secret' }));

  const stored: unknown = JSON.parse(storage.values[LIST_KEY] ?? '[]');
  assert.deepEqual(stored, [acme]);
  for (const entry of stored as string[]) {
    const payload: unknown = JSON.parse(Buffer.from(entry.split('.')[0] ?? '', 'base64url').toString());
    assert.deepEqual(Object.keys(payload as object).sort(), ['host', 'name']);
  }
  assert.deepEqual(list.all(), [{ entry: acme, host: 'acme.hub.example', name: 'Acme Ltd' }]);
});

test('one entry per host, the newest first; forgetting takes it off at once (check 4)', () => {
  const list = entryList(fakeStorage());
  list.keep(acme);
  list.keep(meier);
  const renamed = signed({ host: 'acme.hub.example', name: 'Acme Group' });
  list.keep(renamed);
  assert.deepEqual(list.all().map((each) => each.name), ['Acme Group', 'Meier family']);

  list.forget('acme.hub.example');
  assert.deepEqual(list.all().map((each) => each.host), ['meier.hub.example']);
  list.drop(meier);
  assert.deepEqual(list.all(), []);
});

test('what is not shaped as an entry is never read as one', () => {
  for (const each of ['', 'no-dot', 'a.b.c', signed({ host: 'https://acme.example', name: 'A' }), signed({ host: 'acme.example', name: ' ' }), signed(['acme.example', 'A'])]) {
    assert.equal(readEntry(each), undefined, each);
  }
});

test('a copy handed over replaces the list, and a storage that refuses keeps none', () => {
  const list = entryList(fakeStorage());
  list.keep(acme);
  list.replace([meier, 'planted']);
  assert.deepEqual(list.all().map((each) => each.host), ['meier.hub.example']);

  const refusing = entryList({
    getItem: () => {
      throw new Error('refused');
    },
    setItem: () => {
      throw new Error('refused');
    },
    removeItem: () => {
      throw new Error('refused');
    },
  });
  refusing.keep(acme);
  assert.deepEqual(refusing.all(), []);
});

test('the redirects there and back carry their change in the fragment, and come back only to a path', () => {
  assert.deepEqual(readHop(hopFragment({ kind: 'add', entry: acme, path: '/collections/1?item=2' })), {
    kind: 'add', entry: acme, path: '/collections/1?item=2',
  });
  assert.deepEqual(readHop(hopFragment({ kind: 'forget', host: 'meier.hub.example', back: acme, path: '/' })), {
    kind: 'forget', host: 'meier.hub.example', back: acme, path: '/',
  });
  assert.deepEqual(readHop(hopFragment({ kind: 'open', host: 'meier.hub.example' })), { kind: 'open', host: 'meier.hub.example' });
  // Another origin is never a path to come back to.
  assert.deepEqual(readHop('add=x&path=%2F%2Fevil.example'), { kind: 'add', entry: 'x', path: '/' });
  assert.deepEqual(readHop('add=x&path=https%3A%2F%2Fevil.example'), { kind: 'add', entry: 'x', path: '/' });
  assert.equal(readHop('#token=abc'), undefined);

  assert.deepEqual(readHandover(handoverFragment([acme, meier])), [acme, meier]);
  assert.deepEqual(readHandover(handoverFragment([])), []);
  assert.equal(readHandover('#token=abc'), undefined);

  assert.equal(addressOf('acme.hub.example', { protocol: 'https:', port: '' }), 'https://acme.hub.example');
  assert.equal(addressOf('acme.localhost', { protocol: 'http:', port: '5173' }), 'http://acme.localhost:5173');
});
