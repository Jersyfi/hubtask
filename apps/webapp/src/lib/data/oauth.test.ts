// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// What the address means, and where the browser goes afterwards. Both are decided before any
// request leaves, which is why they are tested without one.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { completionUrl, declineUrl, readRequest } from './oauth.ts';

const full =
  '?client_id=c1&redirect_uri=https%3A%2F%2Fapp.example%2Fcb&scope=items%3Aread%20items%3Awrite' +
  '&code_challenge=abc&code_challenge_method=S256&state=xyz';

test('a complete request is read whole', () => {
  const request = readRequest(full);
  assert.deepEqual(request, {
    clientId: 'c1',
    redirectUri: 'https://app.example/cb',
    scopes: ['items:read', 'items:write'],
    challenge: 'abc',
    method: 'S256',
    state: 'xyz',
  });
});

test('the four mandatory fields are what makes it a request', () => {
  // Anything missing is "there is no request here" — a bookmark, a reload — which the screen says
  // differently from "the server refused this one".
  for (const missing of ['client_id', 'redirect_uri', 'code_challenge', 'scope']) {
    const query = new URLSearchParams(full.slice(1));
    query.delete(missing);
    assert.equal(readRequest(`?${query}`), undefined, `${missing} should be required`);
  }
  assert.equal(readRequest(''), undefined);
});

test('the method travels as it arrived rather than being defaulted', () => {
  // The server refuses anything but S256. Defaulting it here would make a request that is about to
  // be refused look valid on the way in.
  const query = new URLSearchParams(full.slice(1));
  query.delete('code_challenge_method');
  assert.equal(readRequest(`?${query}`)?.method, '');
  query.set('code_challenge_method', 'plain');
  assert.equal(readRequest(`?${query}`)?.method, 'plain');
});

test('a state the app did not send is absent rather than empty', () => {
  const query = new URLSearchParams(full.slice(1));
  query.delete('state');
  assert.equal(readRequest(`?${query}`)?.state, undefined);
});

test('the code and the state are appended to the URI the server validated', () => {
  assert.equal(
    completionUrl('https://app.example/cb', 'the-code', 'xyz'),
    'https://app.example/cb?code=the-code&state=xyz',
  );
  // An app that sent no state gets none back rather than an empty one.
  assert.equal(completionUrl('https://app.example/cb', 'the-code', undefined), 'https://app.example/cb?code=the-code');
  // A URI that already carries a query keeps it.
  assert.equal(
    completionUrl('https://app.example/cb?a=1', 'the-code', undefined),
    'https://app.example/cb?a=1&code=the-code',
  );
});

test('declining tells the app in its own vocabulary', () => {
  // RFC 6749 §4.1.2.1. Better than leaving the app waiting, and not a sentence this client invents.
  assert.equal(
    declineUrl('https://app.example/cb', 'xyz'),
    'https://app.example/cb?error=access_denied&state=xyz',
  );
});

test('anything that is not a navigation is refused', () => {
  // The server has matched the URI byte for byte against the registration, so this is not distrust
  // of the answer: it is that `location.assign` is where a string becomes executable.
  for (const hostile of ['javascript:alert(1)', 'data:text/html,<script>', '/relative', '']) {
    assert.equal(completionUrl(hostile, 'c', undefined), undefined);
    assert.equal(declineUrl(hostile, undefined), undefined);
  }
});
