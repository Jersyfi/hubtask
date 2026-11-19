// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { DOCUMENT_SERVER_KINDS, choiceOf, isTestable, movedFrom } from './documentServer.ts';

// UC-MED-06 check 1: none and the three servers attached through WOPI are offered; the largest
// vendor's browser office stays a choice of the contract, not of this page.
test('the administration chooses none or one of the three servers', () => {
  assert.deepEqual([...DOCUMENT_SERVER_KINDS], ['NONE', 'COLLABORA_ONLINE', 'ONLYOFFICE', 'EURO_OFFICE']);
});

test('a choice sends an address only with a server, without the trailing slash', () => {
  assert.deepEqual(choiceOf('NONE', 'https://office.example'), { kind: 'NONE' });
  assert.deepEqual(choiceOf('COLLABORA_ONLINE', ' https://office.example/ '), { kind: 'COLLABORA_ONLINE', url: 'https://office.example' });
});

test('only a server with an address can be tested', () => {
  assert.equal(isTestable('NONE', 'https://office.example'), false);
  assert.equal(isTestable('ONLYOFFICE', '  '), false);
  assert.equal(isTestable('ONLYOFFICE', 'https://office.example'), true);
});

test('what moved is counted against what is stored, an absent answer as none', () => {
  assert.equal(movedFrom(undefined, 'NONE', ''), 0);
  // An answer without a kind (a proxy's page, an older server) is none, not a change to save.
  assert.equal(movedFrom({} as never, 'NONE', ''), 0);
  assert.equal(movedFrom({ kind: 'NONE' }, 'EURO_OFFICE', 'https://office.example'), 2);
  assert.equal(movedFrom({ kind: 'COLLABORA_ONLINE', url: 'https://office.example' }, 'COLLABORA_ONLINE', 'https://office.example/'), 0);
  assert.equal(movedFrom({ kind: 'COLLABORA_ONLINE', url: 'https://office.example' }, 'ONLYOFFICE', 'https://office.example'), 1);
  // Choosing none clears the address, whatever was typed into the hidden field.
  assert.equal(movedFrom({ kind: 'NONE' }, 'NONE', 'https://typed.example'), 0);
});
