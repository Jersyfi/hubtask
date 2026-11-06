// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { addressOf, consentUrl, isOlder, judge, loopbackRedirect, newPkce } from './appSignIn.ts';

test('an address keeps only its host, over HTTPS', () => {
  const cases: Array<[string, string | undefined]> = [
    ['tasks.acme.com', 'https://tasks.acme.com'],
    ['  Tasks.Acme.com ', 'https://tasks.acme.com'],
    ['https://tasks.acme.com/sign-in?x=1', 'https://tasks.acme.com'],
    ['tasks.acme.com:8443', 'https://tasks.acme.com:8443'],
    ['localhost:8080', 'http://localhost:8080'],
    ['http://127.0.0.1:8080', 'http://127.0.0.1:8080'],
    ['http://tasks.acme.com', undefined],
    ['ftp://tasks.acme.com', undefined],
    ['user@tasks.acme.com', undefined],
    ['tasks', undefined],
    ['tasks acme', undefined],
    ['', undefined],
  ];
  for (const [typed, want] of cases) assert.equal(addressOf(typed)?.origin, want, typed);
});

const ACME = addressOf('tasks.acme.com')!;
const SELF = { product_version: '1.2.0', tenancy_mode: 'single', app_client_id: 'client' };

test('each refusal of an address has its reason', () => {
  assert.deepEqual(judge(ACME, undefined, '1.2.0'), { refused: { kind: 'unreachable' } });
  assert.deepEqual(judge(ACME, null, '1.2.0'), { refused: { kind: 'not-hubtask' } });
  assert.deepEqual(judge(ACME, {}, '1.2.0'), { refused: { kind: 'not-hubtask' } });
  assert.deepEqual(judge(ACME, { ...SELF, product_version: '1.1.4' }, '1.2.0'), {
    refused: { kind: 'older', server: '1.1.4', app: '1.2.0' },
  });
  // A server from before the built-in client cannot be signed in to by the app.
  assert.deepEqual(judge(ACME, { product_version: '1.2.0' }, '1.2.0'), {
    refused: { kind: 'older', server: '1.2.0', app: '1.2.0' },
  });
  assert.deepEqual(
    judge(ACME, { ...SELF, tenancy_mode: 'multi', installation_url: 'https://tasks.acme.com' }, '1.2.0'),
    { refused: { kind: 'installation' } },
  );
  assert.deepEqual(
    judge(ACME, { ...SELF, tenancy_mode: 'multi', installation_url: 'https://example.net' }, '1.2.0'),
    { client: 'client' },
  );
  assert.deepEqual(judge(ACME, SELF, '1.2.0'), { client: 'client' });
});

test('versions compare by their release parts', () => {
  assert.equal(isOlder('1.1.4', '1.2.0'), true);
  assert.equal(isOlder('1.2.0', '1.2.0'), false);
  assert.equal(isOlder('1.10.0', '1.9.3'), false);
  assert.equal(isOlder('2.0.0-rc.1', '1.9.0'), false);
  assert.equal(isOlder('0.9.5', '0.0.0'), false);
});

test('the consent asks for the built-in client with S256 and the device', async () => {
  const pkce = await newPkce((bytes) => bytes.fill(7));
  assert.equal(pkce.verifier.length, 43);
  assert.equal(pkce.challenge.length, 43);
  const url = new URL(consentUrl(ACME, {
    client: 'client', redirect: loopbackRedirect(51734), challenge: pkce.challenge,
    state: 'state', device: 'LENA-LAPTOP', platform: 'windows',
  }));
  assert.equal(url.origin, 'https://tasks.acme.com');
  assert.equal(url.pathname, '/oauth/consent');
  assert.equal(url.searchParams.get('redirect_uri'), 'http://127.0.0.1:51734/oauth');
  assert.equal(url.searchParams.get('code_challenge_method'), 'S256');
  assert.equal(url.searchParams.get('device'), 'LENA-LAPTOP');
  assert.equal(url.searchParams.get('scope'), null);
  assert.doesNotMatch(url.href, new RegExp(pkce.verifier));
});
