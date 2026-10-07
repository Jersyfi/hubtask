// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The recovery a privileged refusal asks for: try, ask once, retry once, never twice.
//
// What these tests are actually for is the "never twice": a dialog that reopens on the answer it
// just produced is a dialog somebody cannot escape, and that is the failure mode a wrapper like
// this has.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { TransportError } from '@hubtask/sync-engine';

import {
  heldGrant,
  holdGrant,
  isReturning,
  methodsOf,
  providerOf,
  rememberReturn,
  takeGrant,
  takeReturn,
  withStepUp,
} from './stepup.ts';

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
  // No list at all is an older server, which named the password for everybody.
  assert.deepEqual(methodsOf(refusal()), ['PASSWORD']);
  // An empty list is an answer: an account that signs in only through a provider and has no
  // factor can prove itself with nothing here, and a password field would be one it cannot fill
  // (UC-ID-05 check 5).
  assert.deepEqual(methodsOf(refusal({ methods: '' })), []);
  assert.deepEqual(methodsOf(refusal({ methods: '   ' })), []);
  // A method this client cannot render is dropped rather than shown as a field nobody can fill.
  assert.deepEqual(methodsOf(refusal({ methods: 'WEBAUTHN' })), ['PASSWORD']);
  assert.deepEqual(methodsOf(refusal({ methods: 'WEBAUTHN,TOTP' })), ['TOTP']);
});

test('the four methods are read, and the provider by its name', () => {
  // ADR-0075 §1: the account's own ways, in the server's order.
  assert.deepEqual(
    methodsOf(refusal({ methods: 'PASSWORD TOTP RECOVERY PROVIDER', provider: 'Contoso Entra ID' })),
    ['PASSWORD', 'TOTP', 'RECOVERY', 'PROVIDER'],
  );
  assert.deepEqual(methodsOf(refusal({ methods: 'PROVIDER' })), ['PROVIDER']);
  assert.equal(providerOf(refusal({ methods: 'PROVIDER', provider: 'Contoso Entra ID' })), 'Contoso Entra ID');
  assert.equal(providerOf(refusal({ methods: 'PASSWORD' })), undefined);
});

test('a held grant answers the first call that asks for a proof, and only one that asks', async () => {
  // The provider's round trip leaves the page: the grant it answered is held until the person does
  // what they were doing again - and a call that needs no proof does not spend it.
  let taken = 0;
  const held = () => {
    taken += 1;
    return 'held-grant';
  };
  const plain = await withStepUp(async () => 'plain', async () => 'asked', held);
  assert.equal(plain, 'plain');
  assert.equal(taken, 0, 'a call that needed no proof took the held grant');

  const presented: (string | undefined)[] = [];
  let asked = 0;
  const answer = await withStepUp(
    async (token) => {
      presented.push(token);
      if (token === undefined) throw refusal({ methods: 'PROVIDER' });
      return 'done';
    },
    async () => {
      asked += 1;
      return 'grant';
    },
    held,
  );
  assert.equal(answer, 'done');
  assert.deepEqual(presented, [undefined, 'held-grant']);
  assert.equal(asked, 0);
});

test('a held grant the server refuses falls back to asking', async () => {
  const presented: (string | undefined)[] = [];
  await withStepUp(
    async (token) => {
      presented.push(token);
      if (token !== 'fresh') throw refusal({ methods: 'TOTP' });
      return 'done';
    },
    async () => 'fresh',
    () => 'stale-grant',
  );
  assert.deepEqual(presented, [undefined, 'stale-grant', 'fresh']);
});

/** A Storage the module can be handed, without a browser. */
function memory(): Pick<Storage, 'getItem' | 'setItem' | 'removeItem'> {
  const held = new Map<string, string>();
  return {
    getItem: (key) => held.get(key) ?? null,
    setItem: (key, value) => void held.set(key, value),
    removeItem: (key) => void held.delete(key),
  };
}

test('the way back is remembered across the provider and taken once', () => {
  const store = memory();
  const now = Date.parse('2026-10-01T12:00:00Z');
  rememberReturn(store, '/administration/sign-in', 'Contoso Entra ID', now);
  assert.equal(isReturning(store, now + 60_000), true);
  assert.deepEqual(takeReturn(store), { returnTo: '/administration/sign-in', provider: 'Contoso Entra ID' });
  assert.equal(takeReturn(store), undefined, 'a second callback is not a step-up');
  // Only a path of this application: a value that named another origin would be a redirect.
  rememberReturn(store, 'https://elsewhere.example/', 'X', now);
  assert.equal(takeReturn(store)?.returnTo, '/');

  // A trip abandoned with Back is not a step-up an hour later: the tab's next provider sign-in is a
  // sign-in, and the stale note is gone.
  rememberReturn(store, '/administration/sign-in', 'Contoso Entra ID', now);
  assert.equal(isReturning(store, now + 11 * 60_000), false);
  assert.equal(takeReturn(store), undefined);
});

test('a held grant lives until it is used or its window ends', () => {
  const store = memory();
  const now = Date.parse('2026-10-01T12:00:00Z');
  holdGrant(store, { token: 'hbt_sup_x', expiresAt: '2026-10-01T12:05:00Z', provider: 'Contoso' });
  assert.equal(heldGrant(store, now)?.provider, 'Contoso');
  assert.equal(takeGrant(store, now), 'hbt_sup_x');
  assert.equal(takeGrant(store, now), undefined, 'one action consumes it');

  holdGrant(store, { token: 'hbt_sup_y', expiresAt: '2026-10-01T12:05:00Z', provider: 'Contoso' });
  assert.equal(takeGrant(store, now + 6 * 60_000), undefined, 'a grant past its window is not offered');
  assert.equal(heldGrant(store, now), undefined, 'and is forgotten');
});
