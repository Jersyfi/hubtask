// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The browser's half of adding a passkey: the bytes carried across between WebAuthn's JSON form and
// the browser's buffers, and the few things the screen decides from the browser.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { PasskeyList, PasskeyRegistrationOptions } from '@hubtask/sync-engine';

import {
  assertionOf,
  canCreatePasskeys,
  canOfferInField,
  ceremonyEndOf,
  creationOptionsOf,
  fromBase64Url,
  isAtRelyingParty,
  isStale,
  keyAnswerOf,
  keyRequestOptionsOf,
  nameFits,
  registrationOf,
  removalOf,
  requestOptionsOf,
  suggestedName,
  toBase64Url,
} from './passkeys.ts';

test('base64url carries every byte both ways, without padding', () => {
  const bytes = new Uint8Array([0, 1, 62, 63, 250, 251, 255]);
  const text = toBase64Url(bytes);
  assert.equal(text.includes('='), false);
  assert.equal(/[+/]/.test(text), false);
  assert.deepEqual([...fromBase64Url(text)], [...bytes]);
  assert.deepEqual([...fromBase64Url('AQID')], [1, 2, 3]);
  assert.deepEqual([...fromBase64Url('AQ==')], [1]);
  assert.equal(toBase64Url(new Uint8Array([]).buffer), '');
});

const options: PasskeyRegistrationOptions = {
  expires_at: '2026-10-16T09:05:00Z',
  public_key: {
    rp: { id: 'acme.hubtask.example', name: 'Acme' },
    user: { id: 'MzMz', name: 'bert@example.org', displayName: 'Bert' },
    challenge: 'KioqKg',
    pubKeyCredParams: [
      { type: 'public-key', alg: -7 },
      { type: 'public-key', alg: -8 },
      { type: 'public-key', alg: -257 },
    ],
    timeout: 300000,
    excludeCredentials: [{ type: 'public-key', id: 'AQ', transports: ['usb'] }, { type: 'public-key', id: 'Ag' }],
    authenticatorSelection: { residentKey: 'preferred', requireResidentKey: false, userVerification: 'preferred' },
    attestation: 'none',
  },
};

test('the server’s options become the browser’s, buffers where WebAuthn wants buffers', () => {
  const created = creationOptionsOf(options);
  assert.equal(created.rp.id, 'acme.hubtask.example');
  assert.deepEqual([...new Uint8Array(created.user.id as ArrayBuffer)], [51, 51, 51]);
  assert.deepEqual([...new Uint8Array(created.challenge as ArrayBuffer)], [42, 42, 42, 42]);
  assert.deepEqual(created.pubKeyCredParams.map((param) => param.alg), [-7, -8, -257]);
  assert.equal(created.excludeCredentials?.length, 2);
  assert.deepEqual(created.excludeCredentials?.[0]?.transports, ['usb']);
  assert.equal('transports' in (created.excludeCredentials?.[1] ?? {}), false);
  assert.equal(created.authenticatorSelection?.userVerification, 'preferred');
  assert.equal(created.attestation, 'none');
});

test('the ceremony’s answer is encoded, the name trimmed, unknown transports left out', () => {
  const registration = registrationOf(
    {
      rawId: new Uint8Array([1, 2]).buffer,
      response: {
        clientDataJSON: new Uint8Array([123, 125]).buffer,
        attestationObject: new Uint8Array([160]).buffer,
        getTransports: () => ['internal', 'hybrid', 'internal', 'carrier-pigeon'],
      },
    },
    '  iPhone ',
  );
  assert.deepEqual(registration, {
    name: 'iPhone',
    raw_id: 'AQI',
    client_data_json: 'e30',
    attestation_object: 'oA',
    transports: ['internal', 'hybrid'],
  });
  const without = registrationOf(
    { rawId: new ArrayBuffer(1), response: { clientDataJSON: new ArrayBuffer(1), attestationObject: new ArrayBuffer(1) } },
    'Key',
  );
  assert.equal('transports' in without, false);
});

test('a passkey can be added only by a browser that can create one, at the relying party’s host', () => {
  assert.equal(canCreatePasskeys({ PublicKeyCredential: class {}, navigator: { credentials: { create: () => undefined } } }), true);
  assert.equal(canCreatePasskeys({ navigator: { credentials: { create: () => undefined } } }), false);
  assert.equal(canCreatePasskeys({ PublicKeyCredential: class {}, navigator: {} }), false);

  assert.equal(isAtRelyingParty('acme.hubtask.example', 'acme.hubtask.example'), true);
  assert.equal(isAtRelyingParty('acme.hubtask.example', 'ACME.hubtask.example'), true);
  // Multi mode, the page at the installation's host: the browser would refuse the ceremony.
  assert.equal(isAtRelyingParty('acme.hubtask.example', 'hubtask.example'), false);
  assert.equal(isAtRelyingParty(null, 'acme.hubtask.example'), false);
  assert.equal(isAtRelyingParty('', ''), false);
});

test('a prompt the person closes is no failure; an authenticator already holding one is said so', () => {
  assert.equal(ceremonyEndOf(new DOMException('closed', 'NotAllowedError')), 'cancelled');
  assert.equal(ceremonyEndOf(new DOMException('aborted', 'AbortError')), 'cancelled');
  assert.equal(ceremonyEndOf(new DOMException('held', 'InvalidStateError')), 'held');
  assert.equal(ceremonyEndOf(new DOMException('nope', 'SecurityError')), 'failed');
  assert.equal(ceremonyEndOf('a string'), 'failed');
});

test('the suggested name counts on past the names already taken', () => {
  assert.equal(suggestedName('Passkey', []), 'Passkey 1');
  assert.equal(suggestedName('Passkey', [{ name: 'iPhone' }]), 'Passkey 2');
  assert.equal(suggestedName('Passkey', [{ name: 'Passkey 2' }]), 'Passkey 3');
});

test('a name fits as the server counts it: code points after trimming, never blank', () => {
  assert.equal(nameFits('  Laptop at work '), true);
  assert.equal(nameFits('   '), false);
  assert.equal(nameFits('ä'.repeat(64)), true);
  assert.equal(nameFits('ä'.repeat(65)), false);
  // Sixty-four emoji are 128 UTF-16 units and still a name that fits.
  assert.equal(nameFits('🔑'.repeat(64)), true);
  assert.equal(nameFits('🔑'.repeat(65)), false);
});

const list: PasskeyList = { passkeys: [], limit: 10, can_add: true, can_remove: true, rp_id: 'acme.hubtask.example' };

test('a passkey of another address is stale; one without a recorded host is not', () => {
  assert.equal(isStale({ rp_id: 'old.acme.example' }, list), true);
  assert.equal(isStale({ rp_id: 'acme.hubtask.example' }, list), false);
  assert.equal(isStale({ rp_id: null }, list), false);
});

test('Remove is offered exactly where the server would let it pass, and the reason stands in its place', () => {
  const current = { rp_id: 'acme.hubtask.example' };
  const stale = { rp_id: 'old.acme.example' };
  assert.deepEqual(removalOf(current, list), { offered: true });
  const lastRequired: PasskeyList = { ...list, can_remove: false, cannot_remove_reason: 'auth.mfa_required_by_tenant' };
  assert.deepEqual(removalOf(current, lastRequired), { offered: false, reason: 'auth.mfa_required_by_tenant' });
  assert.deepEqual(removalOf(stale, lastRequired), { offered: true }, 'a stale key is no factor and always goes');
  const noProof: PasskeyList = { ...list, can_remove: false, cannot_remove_reason: 'auth.step_up_no_method' };
  assert.deepEqual(removalOf(stale, noProof), { offered: false, reason: 'auth.step_up_no_method' });
});

test('the sign-in options become the browser’s, in the field or asked directly', async () => {
  const signal = new AbortController().signal;
  const given = {
    expires_at: '2099-01-01T00:05:00Z',
    public_key: { rpId: 'acme.hubtask.example', challenge: 'KioqKg', timeout: 300000, userVerification: 'required' as const },
  };
  const field = requestOptionsOf(given, 'conditional', signal);
  assert.equal(field.mediation, 'conditional');
  assert.equal(field.signal, signal);
  assert.equal(field.publicKey?.rpId, 'acme.hubtask.example');
  assert.equal(field.publicKey?.userVerification, 'required');
  assert.deepEqual([...new Uint8Array(field.publicKey?.challenge as ArrayBuffer)], [42, 42, 42, 42]);
  assert.equal('allowCredentials' in (field.publicKey ?? {}), false, 'a discoverable sign-in names nobody');
  const button = requestOptionsOf(given, 'optional');
  assert.equal(button.mediation, 'optional');
  assert.equal('signal' in button, false);

  assert.deepEqual(
    assertionOf({
      rawId: new Uint8Array([1, 2]).buffer,
      response: {
        clientDataJSON: new Uint8Array([123, 125]).buffer,
        authenticatorData: new Uint8Array([9]).buffer,
        signature: new Uint8Array([48]).buffer,
        userHandle: new Uint8Array([51, 51]).buffer,
      },
    }),
    { raw_id: 'AQI', client_data_json: 'e30', authenticator_data: 'CQ', signature: 'MA', user_handle: 'MzM' },
  );

  assert.equal(await canOfferInField({}), false);
  assert.equal(await canOfferInField({ PublicKeyCredential: { isConditionalMediationAvailable: async () => true } }), true);
  assert.equal(await canOfferInField({ PublicKeyCredential: { isConditionalMediationAvailable: async () => { throw new Error('no'); } } }), false);
});

test('a known account’s key is asked with its own credentials, and answers without a handle', () => {
  const asked = keyRequestOptionsOf({
    expires_at: '2099-01-01T00:05:00Z',
    public_key: {
      rpId: 'acme.hubtask.example', challenge: 'KioqKg', timeout: 300000, userVerification: 'preferred',
      allowCredentials: [{ type: 'public-key', id: 'C-c', transports: ['usb'] }],
    },
  });
  assert.equal(asked.mediation, 'optional');
  assert.equal(asked.publicKey?.userVerification, 'preferred', 'presence is enough for a second factor');
  const allowed = asked.publicKey?.allowCredentials ?? [];
  assert.equal(allowed.length, 1);
  assert.deepEqual([...new Uint8Array(allowed[0]?.id as ArrayBuffer)], [11, 231]);
  assert.deepEqual(allowed[0]?.transports, ['usb']);

  assert.deepEqual(
    keyAnswerOf({
      rawId: new Uint8Array([1, 2]).buffer,
      response: {
        clientDataJSON: new Uint8Array([123, 125]).buffer,
        authenticatorData: new Uint8Array([9]).buffer,
        signature: new Uint8Array([48]).buffer,
        userHandle: null,
      },
    }),
    { raw_id: 'AQI', client_data_json: 'e30', authenticator_data: 'CQ', signature: 'MA' },
  );
});
