// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { TransportError } from '@hubtask/sync-engine';

import { RESOLVE_ATTEMPTS, ResolveBatch } from './accountbatch.ts';

type Account = { readonly id: string };
const flush = () => new Promise((resolve) => setTimeout(resolve, 0));

function harness(answer: (ids: readonly string[], call: number) => readonly Account[]) {
  const requests: (readonly string[])[] = [];
  const waits: number[] = [];
  const known: string[] = [];
  const unknown: string[] = [];
  const given: string[] = [];
  const batch = new ResolveBatch<Account>({
    read: async (ids) => {
      requests.push(ids);
      return answer(ids, requests.length);
    },
    idOf: (account) => account.id,
    settled: (found, missing) => {
      known.push(...found.map((account) => account.id));
      unknown.push(...missing);
    },
    gaveUp: (ids) => given.push(...ids),
    wait: async (ms) => { waits.push(ms); },
  });
  return { batch, requests, waits, known, unknown, given };
}

test('a turn of 120 names is one request, and 450 are three of at most 200', async () => {
  const ids = Array.from({ length: 120 }, (_, i) => `a-${i}`);
  const one = harness((asked) => asked.map((id) => ({ id })));
  for (const id of ids) one.batch.ask([id]);
  await flush();
  assert.equal(one.requests.length, 1);
  assert.equal(one.known.length, 120);

  const many = harness((asked) => asked.map((id) => ({ id })));
  many.batch.ask(Array.from({ length: 450 }, (_, i) => `b-${i}`));
  await flush();
  assert.deepEqual(many.requests.map((r) => r.length), [200, 200, 50]);
});

test('a throttled request is asked again after Retry-After, and nobody becomes unknown for it', async () => {
  const h = harness((asked, call) => {
    if (call === 1) throw new TransportError('problem', { status: 429, retryAfterMs: 2_000 });
    return asked.map((id) => ({ id }));
  });
  h.batch.ask(['a', 'b']);
  await flush();
  await flush();
  assert.deepEqual(h.waits, [2_000]);
  assert.deepEqual(h.known, ['a', 'b']);
  assert.deepEqual(h.unknown, []);
});

test('only an identifier the answer leaves out, or a refusal, is unknown', async () => {
  const left = harness(() => [{ id: 'a' }]);
  left.batch.ask(['a', 'gone']);
  await flush();
  assert.deepEqual(left.unknown, ['gone']);

  const refused = harness(() => { throw new TransportError('problem', { status: 403 }); });
  refused.batch.ask(['x']);
  await flush();
  assert.deepEqual(refused.unknown, ['x']);
  assert.deepEqual(refused.waits, []);
});

test('a server that stays down is given up for now, not marked unknown', async () => {
  const h = harness(() => { throw new TransportError('problem', { status: 503 }); });
  h.batch.ask(['a']);
  for (let i = 0; i < RESOLVE_ATTEMPTS + 2; i += 1) await flush();
  assert.equal(h.requests.length, RESOLVE_ATTEMPTS);
  assert.deepEqual(h.given, ['a']);
  assert.deepEqual(h.unknown, []);
});
