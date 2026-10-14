// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { IdentityProviderPreset } from '@hubtask/sync-engine';

import { issuerAfterChoosing } from './issuer.ts';

const GOOGLE = 'https://accounts.google.com';
const presets = [
  { kind: 'GOOGLE', issuer: GOOGLE },
  { kind: 'MICROSOFT' },
  { kind: 'GENERIC' },
] as unknown as IdentityProviderPreset[];

test('choosing the provider with one issuer fills it into an empty field', () => {
  assert.equal(issuerAfterChoosing(presets, 'GOOGLE', ''), GOOGLE);
});

test('what somebody typed stays, whichever provider is chosen', () => {
  const own = 'https://login.microsoftonline.com/1234/v2.0';
  assert.equal(issuerAfterChoosing(presets, 'GOOGLE', own), own);
  assert.equal(issuerAfterChoosing(presets, 'GENERIC', own), own);
});

test("another provider's filled issuer is taken out again", () => {
  assert.equal(issuerAfterChoosing(presets, 'MICROSOFT', GOOGLE), '');
  assert.equal(issuerAfterChoosing(presets, 'GOOGLE', GOOGLE), GOOGLE);
});
