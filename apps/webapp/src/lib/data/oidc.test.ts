// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// What the address means, and what may be navigated to. Both are decisions made before any
// request leaves, which is why they are tested without one.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { navigableUrl, readArrival } from './oidc.ts';

test('a code and a state are the handoff', () => {
  const arrival = readArrival('?code=abc&state=xyz');
  assert.equal(arrival.kind, 'handoff');
  assert.deepEqual(arrival.kind === 'handoff' ? arrival.handoff : undefined, {
    code: 'abc',
    state: 'xyz',
  });
});

test('the leading question mark is optional', () => {
  assert.equal(readArrival('code=abc&state=xyz').kind, 'handoff');
});

test('a provider that refused says so without a code', () => {
  // The person pressed cancel. There is nothing to exchange, so nothing is sent.
  assert.equal(readArrival('?error=access_denied&state=xyz').kind, 'refused');
  assert.equal(readArrival('?error=access_denied').kind, 'refused');
});

test('a state without a code is not a handoff', () => {
  // An empty code would be asking the server to refuse what is visibly not there.
  assert.equal(readArrival('?state=xyz').kind, 'nothing');
  assert.equal(readArrival('?code=abc').kind, 'nothing');
  assert.equal(readArrival('').kind, 'nothing');
});

test('an empty parameter is an absent one', () => {
  assert.equal(readArrival('?code=&state=xyz').kind, 'nothing');
});

test('a provider’s http or https address may be navigated to', () => {
  assert.equal(
    navigableUrl('https://login.example.com/authorize?client_id=hubtask'),
    'https://login.example.com/authorize?client_id=hubtask',
  );
  // A development provider on a machine with no certificate. The server decides what it answers;
  // this only decides what a browser may be sent to.
  assert.equal(navigableUrl('http://localhost:8080/authorize'), 'http://localhost:8080/authorize');
});

test('anything that is not a navigation is refused', () => {
  // The one that matters: `location.assign` would run this in this origin.
  assert.equal(navigableUrl('javascript:alert(1)'), undefined);
  assert.equal(navigableUrl('data:text/html,<script>'), undefined);
  assert.equal(navigableUrl('/authorize'), undefined, 'a relative path is not a provider');
  assert.equal(navigableUrl(''), undefined);
});
