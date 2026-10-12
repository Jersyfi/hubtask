// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { passwordFormOf } from './passwordForm.ts';

test('with the password on, the first field takes an address, or a name where a managed account exists', () => {
  assert.equal(passwordFormOf({ hasPassword: true, signInNames: false }), 'email');
  assert.equal(passwordFormOf({ hasPassword: true, signInNames: true }), 'identifier');
});

test('with the password off, a sign-in name keeps a form of its own where a managed account exists', () => {
  assert.equal(passwordFormOf({ hasPassword: false, signInNames: true }), 'name');
});

test('with the password off and no managed account, there is no password form at all', () => {
  assert.equal(passwordFormOf({ hasPassword: false, signInNames: false }), 'none');
});
