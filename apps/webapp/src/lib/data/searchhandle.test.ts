// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import test from 'node:test';
import assert from 'node:assert/strict';
import { webcrypto } from 'node:crypto';

import { HANDLE, isHandle, keyFor, keysIn, mint } from './searchhandle.ts';

test('a handle is eight hex characters, and two of them differ', () => {
  const one = mint((bytes) => webcrypto.getRandomValues(bytes));
  const two = mint((bytes) => webcrypto.getRandomValues(bytes));
  assert.match(one, /^[0-9a-f]{8}$/);
  assert.notEqual(one, two, 'two handles in a row were the same');
});

test('a handle says nothing about what it names', () => {
  // The same words twice produce different handles: a handle derived from the term would let
  // anybody holding the link confirm a guess by computing it again.
  const first = mint((bytes) => webcrypto.getRandomValues(bytes));
  const second = mint((bytes) => webcrypto.getRandomValues(bytes));
  assert.notEqual(first, second);
  assert.equal(HANDLE, 's', 'the address names it `s`');
});

test('what the address may name, and what it may not', () => {
  assert.equal(isHandle('0a1b2c3d'), true);
  assert.equal(isHandle('0A1B2C3D'), false, 'upper case is a second spelling of one handle');
  assert.equal(isHandle('0a1b2c3'), false, 'too short');
  assert.equal(isHandle('0a1b2c3d4'), false, 'too long');
  assert.equal(isHandle('milk'), false, 'a term someone put there by hand is not a handle');
  assert.equal(isHandle(undefined), false);
});

test('the keys are prefixed, so sign-out can find every one', () => {
  assert.equal(keyFor('0a1b2c3d'), 'hubtask.search.0a1b2c3d');

  const held = new Map([
    ['hubtask.search.0a1b2c3d', 'milk'],
    ['hubtask.search.4e5f6a7b', 'bread'],
    ['hubtask.bearer', 'not this one'],
    ['something.else', 'nor this'],
  ]);
  const keys = [...held.keys()];
  const storage = { length: keys.length, key: (index: number) => keys[index] ?? null };

  assert.deepEqual([...keysIn(storage)].sort(), ['hubtask.search.0a1b2c3d', 'hubtask.search.4e5f6a7b']);
});
