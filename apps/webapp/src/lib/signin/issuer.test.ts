// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { IdentityProviderPreset } from '@hubtask/sync-engine';

import { admissionLists, isSharedIssuer, issuerAfterChoosing } from './issuer.ts';

const MICROSOFT = {
  kind: 'MICROSOFT', directory_claim: 'tid', shared_issuer_paths: ['common', 'organizations'],
} as unknown as IdentityProviderPreset;
const GENERIC = { kind: 'GENERIC', shared_issuer_paths: [] } as unknown as IdentityProviderPreset;

test("a shared endpoint is told by its first path segment, from the preset's own list", () => {
  assert.equal(isSharedIssuer(MICROSOFT, 'https://login.microsoftonline.com/organizations/v2.0'), true);
  assert.equal(isSharedIssuer(MICROSOFT, ' https://login.microsoftonline.com/Common/v2.0 '), true);
  assert.equal(isSharedIssuer(MICROSOFT, 'https://login.microsoftonline.com/72f988bf-86f1-41af-91ab-2d7cd011db47/v2.0'), false);
  assert.equal(isSharedIssuer(MICROSOFT, 'https://login.microsoftonline.com/organizationsx/v2.0'), false);
  assert.equal(isSharedIssuer(MICROSOFT, 'not a url'), false);
  assert.equal(isSharedIssuer(GENERIC, 'https://idp.example/organizations'), false);
  assert.equal(isSharedIssuer(undefined, 'https://login.microsoftonline.com/organizations/v2.0'), false);
});

test('the form draws the list the server reads: on a shared endpoint the directories, required, in every mode', () => {
  assert.deepEqual(admissionLists(MICROSOFT, 'ANY', true), { directories: true, domains: false, required: true });
  assert.deepEqual(admissionLists(MICROSOFT, 'INVITED_ONLY', true), { directories: true, domains: false, required: true });
  // A directory's own issuer under ANY: the server would not read a list, so none is drawn.
  assert.deepEqual(admissionLists(MICROSOFT, 'ANY', false), { directories: false, domains: false, required: false });
  assert.deepEqual(admissionLists(MICROSOFT, 'INVITED_ONLY', false), { directories: true, domains: false, required: false });
  assert.deepEqual(admissionLists(MICROSOFT, 'DOMAINS', false), { directories: true, domains: false, required: true });
  assert.deepEqual(admissionLists(GENERIC, 'DOMAINS', false), { directories: false, domains: true, required: true });
  assert.deepEqual(admissionLists(GENERIC, 'ANY', false), { directories: false, domains: false, required: false });
});

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
