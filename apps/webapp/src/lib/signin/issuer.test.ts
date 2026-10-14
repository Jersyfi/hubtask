// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { IdentityProviderPreset } from '@hubtask/sync-engine';

import { admissionLists, isPersonalIssuer, isSharedIssuer, issuerAfterChoosing, offeredModes, personalDirectoryListed } from './issuer.ts';

const PERSONAL_DIRECTORY = '9188040d-6c67-4c5b-b112-36a304b66dad';
const MICROSOFT = {
  kind: 'MICROSOFT', directory_claim: 'tid', shared_issuer_paths: ['common', 'organizations'],
  personal_issuer_paths: ['common', 'consumers', PERSONAL_DIRECTORY],
  personal_directories: [PERSONAL_DIRECTORY],
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

test("an issuer serving personal accounts is told by its first path segment, from the preset's own list", () => {
  assert.equal(isPersonalIssuer(MICROSOFT, 'https://login.microsoftonline.com/consumers/v2.0'), true);
  assert.equal(isPersonalIssuer(MICROSOFT, ' https://login.microsoftonline.com/Common/v2.0 '), true);
  assert.equal(isPersonalIssuer(MICROSOFT, `https://login.microsoftonline.com/${PERSONAL_DIRECTORY}/v2.0`), true);
  assert.equal(isPersonalIssuer(MICROSOFT, 'https://login.microsoftonline.com/organizations/v2.0'), false);
  assert.equal(isPersonalIssuer(MICROSOFT, 'https://login.microsoftonline.com/72f988bf-86f1-41af-91ab-2d7cd011db47/v2.0'), false);
  assert.equal(isPersonalIssuer(MICROSOFT, 'not a url'), false);
  assert.equal(isPersonalIssuer(GENERIC, 'https://idp.example/consumers'), false);
});

test('Anyone is not offered where the server refuses it, unless a stored row already has it', () => {
  const modes = ['INVITED_ONLY', 'DOMAINS', 'ANY'] as const;
  assert.deepEqual(offeredModes(modes, true, 'INVITED_ONLY'), ['INVITED_ONLY', 'DOMAINS']);
  assert.deepEqual(offeredModes(modes, true, 'ANY'), ['INVITED_ONLY', 'DOMAINS', 'ANY']);
  assert.deepEqual(offeredModes(modes, false, 'DOMAINS'), ['INVITED_ONLY', 'DOMAINS', 'ANY']);
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

test('the personal directory is refused in the list wherever the mode creates accounts, compared as the server compares it', () => {
  const organisation = '72f988bf-86f1-41af-91ab-2d7cd011db47';
  // Upper case and surrounding spaces still count: the server lower-cases and trims.
  assert.equal(personalDirectoryListed(MICROSOFT, 'DOMAINS', [organisation, `  ${PERSONAL_DIRECTORY.toUpperCase()}  `]), PERSONAL_DIRECTORY.toUpperCase());
  assert.equal(personalDirectoryListed(MICROSOFT, 'DOMAINS', [PERSONAL_DIRECTORY]), PERSONAL_DIRECTORY);
  assert.equal(personalDirectoryListed(MICROSOFT, 'ANY', [PERSONAL_DIRECTORY]), PERSONAL_DIRECTORY);
  // A narrowing under INVITED_ONLY may name it.
  assert.equal(personalDirectoryListed(MICROSOFT, 'INVITED_ONLY', [PERSONAL_DIRECTORY]), undefined);
  // An organisation's directory, and `common` - an issuer path, which the server saves as a directory.
  assert.equal(personalDirectoryListed(MICROSOFT, 'DOMAINS', [organisation]), undefined);
  assert.equal(personalDirectoryListed(MICROSOFT, 'DOMAINS', ['common', 'consumers']), undefined);
  assert.equal(personalDirectoryListed(MICROSOFT, 'DOMAINS', []), undefined);
  assert.equal(personalDirectoryListed(MICROSOFT, 'DOMAINS', ['', '  ']), undefined);
  // A preset that names none, or none at all.
  assert.equal(personalDirectoryListed(GENERIC, 'DOMAINS', [PERSONAL_DIRECTORY]), undefined);
  assert.equal(personalDirectoryListed(undefined, 'DOMAINS', [PERSONAL_DIRECTORY]), undefined);
});
