// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { PRODUCTS } from '../navigation.ts';
import { isModuleOn, isProductHome, offModuleOf } from './moduleroute.ts';

test('a module page whose module is off says so before it reads anything', () => {
  assert.equal(offModuleOf('service-desk', ['WORKFLOWS']), 'SERVICE_DESK');
  assert.equal(offModuleOf('service-desk', ['SERVICE_DESK', 'WORKFLOWS']), undefined);
  assert.equal(offModuleOf('workflows', []), 'WORKFLOWS');
  assert.equal(offModuleOf('service-desk', undefined), undefined, 'a manifest not read yet withholds nothing');
  assert.equal(offModuleOf('webhooks', []), undefined, 'a page of no module');
  // The approvals waiting for the reader are the desk's, the notes the pages': off, each is refused.
  assert.equal(offModuleOf('approvals', []), 'SERVICE_DESK');
  assert.equal(offModuleOf('notes', ['WORKFLOWS']), 'KNOWLEDGE');
});

test('a module section is drawn only once the module is known to be on', () => {
  assert.equal(isModuleOn('PLANNING', ['PLANNING', 'TIME']), true);
  assert.equal(isModuleOn('PLANNING', ['TIME']), false, 'off: its read would be refused');
  assert.equal(isModuleOn('PLANNING', undefined), false, 'a manifest not read yet asks nothing');
});

// The products band's *Time* and *Goals* open their own pages, not the one home view: reached with
// the module off, Time drew *This screen stopped working* instead of the switch (§2.8).
test('every product\'s home says its module is off, before it reads anything', () => {
  for (const row of PRODUCTS) {
    const home = row.routes[0]!;
    assert.equal(offModuleOf(home, []), row.module, `${row.id} reached with its module off`);
    assert.equal(isProductHome(home), true, `${row.id} is not drawn as a product`);
  }
  assert.equal(isProductHome('service-desk'), false, 'an administration page is not a product home');
});

// A desk is entry types and their workflows: with Workflows off, the desk's pages would ask
// `/entry-types` and be refused, and the home would say no desk takes requests. The page says
// Workflows is off instead, with its switch.
test('a module page whose needed module is off names that one', () => {
  assert.equal(offModuleOf('desk-home', ['SERVICE_DESK']), 'WORKFLOWS');
  assert.equal(offModuleOf('desk-desks', ['SERVICE_DESK']), 'WORKFLOWS');
  assert.equal(offModuleOf('desk-home', ['SERVICE_DESK', 'WORKFLOWS']), undefined);
  assert.equal(offModuleOf('desk-home', ['WORKFLOWS']), 'SERVICE_DESK', 'its own switch first');
});
