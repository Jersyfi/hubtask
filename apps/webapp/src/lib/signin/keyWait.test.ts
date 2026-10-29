// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The wait for a passkey that answers in the system browser (ADR-0096 decision 5).

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { ReturnedCode } from '../platform/index.ts';
import { answerUrl, readRelay } from './keyRelay.ts';
import { KeyWait } from './keyWait.ts';

const OPTIONS = {
  public_key: { rpId: 'tasks.acme.com', challenge: 'Y2hhbGxlbmdl', timeout: 60_000, userVerification: 'preferred' as const, allowCredentials: [] },
  expires_at: '2026-10-29T12:05:00Z',
};
const ANSWER = { raw_id: 'aWQ', client_data_json: 'Y2Q', authenticator_data: 'YWQ', signature: 'c2ln' };

function shell() {
  const opened: string[] = [];
  const waiting: boolean[] = [];
  let returned: ((answer: ReturnedCode) => void) | undefined;
  let retired = 0;
  const wait = new KeyWait({
    signIn: {
      listen: async (_page, deliver) => {
        returned = deliver;
        return 51734;
      },
      retire: () => {
        retired += 1;
      },
      openInBrowser: async (url) => {
        opened.push(url);
      },
    },
    origin: 'https://tasks.acme.com',
    page: () => ({ title: 'Back to the app', body: 'You can close this tab.', lang: 'en' }),
    newState: () => 'state-of-the-dialog',
    onWaiting: (is) => waiting.push(is),
  });
  return { wait, opened, waiting, back: (answer: ReturnedCode) => returned?.(answer), retired: () => retired };
}

/** What the browser's page does with the address it was opened at. */
function browserAnswers(opened: string): ReturnedCode {
  const relay = readRelay(new URL(opened).hash);
  assert.ok(relay);
  const url = new URL(answerUrl(relay, ANSWER));
  return { code: url.searchParams.get('code') ?? undefined, state: url.searchParams.get('state') ?? undefined };
}

test('the browser opens at the workspace host with the options, and its answer is the proof', async () => {
  const { wait, opened, waiting, back } = shell();
  const answering = wait.answer(OPTIONS);
  await new Promise((resolve) => setImmediate(resolve));
  assert.equal(opened.length, 1);
  assert.equal(new URL(opened[0] ?? '').host, 'tasks.acme.com');
  assert.deepEqual(readRelay(new URL(opened[0] ?? '').hash)?.options, OPTIONS);
  back(browserAnswers(opened[0] ?? ''));
  assert.deepEqual(await answering, ANSWER);
  assert.deepEqual(waiting.at(-1), false);
  assert.ok(waiting.includes(true));
});

test('an answer to another request, or a refusal, proves nothing', async () => {
  for (const returned of [{ code: 'x', state: 'somebody-else' }, { error: 'access_denied', state: 'state-of-the-dialog' }]) {
    const { wait, back } = shell();
    const answering = wait.answer(OPTIONS);
    await new Promise((resolve) => setImmediate(resolve));
    back(returned);
    assert.equal(await answering, null);
  }
});

test('Use another way ends the wait and the listener; Open the browser again reopens the same request', async () => {
  const { wait, opened, retired } = shell();
  const answering = wait.answer(OPTIONS);
  await new Promise((resolve) => setImmediate(resolve));
  wait.again();
  await new Promise((resolve) => setImmediate(resolve));
  assert.equal(opened.length, 2);
  assert.equal(opened[1], opened[0]);
  wait.stop();
  assert.equal(await answering, null);
  assert.equal(retired(), 1);
  wait.stop();
  assert.equal(retired(), 1);
});
