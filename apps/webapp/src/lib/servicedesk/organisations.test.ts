// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { domainsOf, offersOrganisation } from './organisations.ts';

test('the mail domains typed become the list the API takes (UC-SVC-11 check 5)', () => {
  assert.deepEqual(domainsOf(' Acme.example, @corp.acme.example\nacme.example '), ['acme.example', 'corp.acme.example']);
  assert.deepEqual(domainsOf('   '), []);
});

test('the portal offers the organisation’s requests only where it shares them (UC-SVC-11 check 5)', () => {
  assert.equal(offersOrganisation({ name: 'Acme', shares_requests: true }), true);
  assert.equal(offersOrganisation({ name: 'Acme', shares_requests: false }), false);
  assert.equal(offersOrganisation(null), false);
  assert.equal(offersOrganisation(undefined), false);
});
