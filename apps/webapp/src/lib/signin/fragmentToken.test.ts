// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { holdFragmentCredentials, takeFragmentToken } from './fragmentToken.ts';

/** A browser's address and history, as far as the two functions read and write them. */
function browserAt(href: string): { address: () => string } {
  let url = new URL(href);
  Object.assign(globalThis, {
    location: {
      get hash() { return url.hash; },
      get pathname() { return url.pathname; },
      get search() { return url.search; },
    },
    history: { replaceState: (_s: unknown, _t: string, to: string) => { url = new URL(to, url); } },
  });
  return { address: () => url.href };
}

// UC-ID-07 check 2: the credential is off the address before the application's first request, and
// the screen that mounts later still receives it.
test('a credential screen loses its token at boot and the screen still takes it once', () => {
  const browser = browserAt('https://contoso.example/redeem#token=hbt_inv_1');
  holdFragmentCredentials();
  assert.equal(browser.address(), 'https://contoso.example/redeem');
  assert.equal(takeFragmentToken(), 'hbt_inv_1');
  assert.equal(takeFragmentToken(), '', 'the token was handed out twice');
});

test('the reset mail’s connect credential is held the same way', () => {
  const browser = browserAt('https://contoso.example/reset?x=1#connect=abc');
  holdFragmentCredentials();
  assert.equal(browser.address(), 'https://contoso.example/reset?x=1');
  assert.equal(takeFragmentToken('connect'), 'abc');
});

test('a fragment on any other screen is left alone', () => {
  const browser = browserAt('https://contoso.example/auth/callback#token=not-ours');
  holdFragmentCredentials();
  assert.equal(browser.address(), 'https://contoso.example/auth/callback#token=not-ours');
});
