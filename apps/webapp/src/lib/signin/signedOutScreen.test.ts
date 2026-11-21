// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { signedOutScreen } from './signedOutScreen.ts';

const unread = { browserSignIn: false, manifestRead: false, manifestFailed: false, setupOpen: false, deviceListHome: false };

test('while the manifest is unread, neither the sign-in nor the setup is chosen', () => {
  assert.equal(signedOutScreen(unread), 'loading');
});

test('a read manifest decides between setup and sign-in', () => {
  assert.equal(signedOutScreen({ ...unread, manifestRead: true, setupOpen: true }), 'setup');
  assert.equal(signedOutScreen({ ...unread, manifestRead: true }), 'sign-in');
  assert.equal(signedOutScreen({ ...unread, manifestRead: true, deviceListHome: true }), 'workspaces');
});

test('a manifest that could not be read leaves the sign-in card, which says why', () => {
  assert.equal(signedOutScreen({ ...unread, manifestFailed: true }), 'sign-in');
});

test('the device list’s home is drawn before the manifest, as it was', () => {
  assert.equal(signedOutScreen({ ...unread, deviceListHome: true }), 'workspaces');
});

test('an installed app signs in through the system browser whatever the manifest says', () => {
  assert.equal(signedOutScreen({ ...unread, browserSignIn: true }), 'app-sign-in');
});
