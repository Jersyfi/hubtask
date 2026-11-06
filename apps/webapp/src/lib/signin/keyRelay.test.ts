// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A passkey step-up from an installed app through the system browser (ADR-0096 decision 5): the
// options out in the fragment, the answer back to the loopback address, and nothing else.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { answerOf, answerUrl, declinedUrl, readRelay, relayUrl, RELAY_PATH } from './keyRelay.ts';

const OPTIONS = {
  public_key: {
    rpId: 'tasks.acme.com',
    challenge: 'c2l4dGVlbi1ieXRlcy1jaGFsbGVuZ2U',
    timeout: 60_000,
    userVerification: 'preferred' as const,
    allowCredentials: [{ type: 'public-key' as const, id: 'a2V5LW9uZQ' }],
  },
  expires_at: '2026-10-29T12:05:00Z',
};
const ANSWER = { raw_id: 'a2V5LW9uZQ', client_data_json: 'eyJ0eXBlIjoid2ViYXV0aG4uZ2V0In0', authenticator_data: 'YXV0aA', signature: 'c2ln' };

test('the page opens at the workspace host, the options in a fragment no request carries', () => {
  const url = new URL(relayUrl('https://tasks.acme.com', { port: 51734, state: 'state-of-the-app', options: OPTIONS }));
  assert.equal(url.origin, 'https://tasks.acme.com');
  assert.equal(url.pathname, RELAY_PATH);
  assert.equal(url.search, '');
  assert.deepEqual(readRelay(url.hash), { port: 51734, state: 'state-of-the-app', options: OPTIONS });
});

test('a fragment that is not the app request is no request', () => {
  const good = new URL(relayUrl('https://tasks.acme.com', { port: 51734, state: 'state-of-the-app', options: OPTIONS })).hash;
  const fields = new URLSearchParams(good.slice(1));
  const without = (key: string, value?: string) => {
    const copy = new URLSearchParams(fields);
    if (value === undefined) copy.delete(key);
    else copy.set(key, value);
    return `#${copy.toString()}`;
  };
  assert.equal(readRelay(''), undefined);
  assert.equal(readRelay(without('port', '0')), undefined);
  assert.equal(readRelay(without('port', '70000')), undefined);
  assert.equal(readRelay(without('port', '80.5')), undefined);
  assert.equal(readRelay(without('state', 'short')), undefined);
  assert.equal(readRelay(without('state', 'a/b?c=dddddddd')), undefined);
  assert.equal(readRelay(without('options', 'not-json')), undefined);
  assert.equal(readRelay(without('options')), undefined);
});

test('the answer goes to the loopback address only, and comes back whole', () => {
  const url = new URL(answerUrl({ port: 51734, state: 'state-of-the-app' }, ANSWER));
  assert.equal(url.origin, 'http://127.0.0.1:51734');
  assert.equal(url.pathname, '/oauth');
  assert.equal(url.searchParams.get('state'), 'state-of-the-app');
  assert.deepEqual(answerOf(url.searchParams.get('code') ?? undefined), ANSWER);
});

test('giving up in the browser tells the app that nothing came', () => {
  const url = new URL(declinedUrl({ port: 51734, state: 'state-of-the-app' }));
  assert.equal(url.origin, 'http://127.0.0.1:51734');
  assert.equal(url.searchParams.get('error'), 'access_denied');
  assert.equal(url.searchParams.get('code'), null);
});

test('a code that is not an answer is refused before anything is sent', () => {
  assert.equal(answerOf(undefined), undefined);
  assert.equal(answerOf('not-base64-json'), undefined);
  const partial = new URL(answerUrl({ port: 1, state: 'state-of-the-app' }, { ...ANSWER, signature: '' })).searchParams.get('code');
  assert.equal(answerOf(partial ?? undefined), undefined);
});
