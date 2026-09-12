// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The recovery a privileged refusal asks for: try, ask once, retry once, never twice.
//
// What these tests are actually for is the "never twice": a dialog that reopens on the answer it
// just produced is a dialog somebody cannot escape, and that is the failure mode a wrapper like
// this has.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { TransportError } from '@hubtask/sync-engine';

import { methodsOf, withStepUp } from './stepup.ts';

function refusal(params?: Record<string, string>): TransportError {
  return new TransportError('problem', {
    status: 403,
    code: 'forbidden',
    detailCode: 'auth.step_up_required',
    params,
  });
}

test('a call that is not refused never asks for a proof', async () => {
  let asked = 0;
  const answer = await withStepUp(
    async () => 'done',
    async () => {
      asked += 1;
      return 'grant';
    },
  );
  assert.equal(answer, 'done');
  assert.equal(asked, 0);
});

test('a refusal asks once, and the retry carries the grant', async () => {
  const presented: (string | undefined)[] = [];
  const answer = await withStepUp(
    async (token) => {
      presented.push(token);
      if (token === undefined) throw refusal();
      return 'done';
    },
    async () => 'grant-1',
  );

  assert.equal(answer, 'done');
  assert.deepEqual(presented, [undefined, 'grant-1'], 'the same call, once without and once with');
});

test('a second refusal after a fresh grant is not asked about again', async () => {
  // The server saying the grant was not what it wanted. A third attempt would be a loop.
  let asked = 0;
  let calls = 0;

  await assert.rejects(
    () =>
      withStepUp(
        async () => {
          calls += 1;
          throw refusal();
        },
        async () => {
          asked += 1;
          return 'grant-1';
        },
      ),
    (error: unknown) => error instanceof TransportError && error.needsStepUp,
  );

  assert.equal(asked, 1, 'asked once');
  assert.equal(calls, 2, 'and attempted twice');
});

test('a prompt the reader closes throws the refusal it came from', async () => {
  // The operation did not happen, and the server's own words say more about it than a sentence
  // this client would invent about cancelling.
  await assert.rejects(
    () =>
      withStepUp(
        async () => {
          throw refusal();
        },
        async () => undefined,
      ),
    (error: unknown) => error instanceof TransportError && error.needsStepUp,
  );
});

test('a refusal that is not about a proof is not a proof question', async () => {
  // An ordinary 403 is a permission. No amount of proving changes it, and a prompt in front of one
  // would be asking somebody to re-type a password to be told no a second time.
  let asked = 0;
  await assert.rejects(
    () =>
      withStepUp(
        async () => {
          throw new TransportError('problem', { status: 403, code: 'forbidden' });
        },
        async () => {
          asked += 1;
          return 'grant';
        },
      ),
    (error: unknown) => error instanceof TransportError && !error.needsStepUp,
  );
  assert.equal(asked, 0);
});

test('the methods offered are the refusal’s, and the password is the floor', () => {
  assert.deepEqual(methodsOf(refusal({ methods: 'TOTP' })), ['TOTP']);
  // The server's own shape, space-separated (stepup.Required, the contract at POST /auth/step-up):
  // the value that was once read as one unknown name and answered with the password alone.
  assert.deepEqual(methodsOf(refusal({ methods: 'PASSWORD TOTP' })), ['PASSWORD', 'TOTP']);
  assert.deepEqual(methodsOf(refusal({ methods: 'PASSWORD' })), ['PASSWORD']);
  assert.deepEqual(methodsOf(refusal({ methods: 'password, totp' })), ['PASSWORD', 'TOTP']);
  // Named nothing: every account has a password, so that is the prompt that always works.
  assert.deepEqual(methodsOf(refusal()), ['PASSWORD']);
  assert.deepEqual(methodsOf(refusal({ methods: '   ' })), ['PASSWORD']);
  // A method this client cannot render is dropped rather than shown as a field nobody can fill.
  assert.deepEqual(methodsOf(refusal({ methods: 'WEBAUTHN' })), ['PASSWORD']);
  assert.deepEqual(methodsOf(refusal({ methods: 'WEBAUTHN,TOTP' })), ['TOTP']);
});
