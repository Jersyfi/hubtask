// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { isModuleOn, offModuleOf } from './moduleroute.ts';

test('a module page whose module is off says so before it reads anything', () => {
  assert.equal(offModuleOf('service-desk', ['WORKFLOWS']), 'SERVICE_DESK');
  assert.equal(offModuleOf('service-desk', ['SERVICE_DESK']), undefined);
  assert.equal(offModuleOf('workflows', []), 'WORKFLOWS');
  assert.equal(offModuleOf('service-desk', undefined), undefined, 'a manifest not read yet withholds nothing');
  assert.equal(offModuleOf('webhooks', []), undefined, 'a page of no module');
});

test('a module section is drawn only once the module is known to be on', () => {
  assert.equal(isModuleOn('PLANNING', ['PLANNING', 'TIME']), true);
  assert.equal(isModuleOn('PLANNING', ['TIME']), false, 'off: its read would be refused');
  assert.equal(isModuleOn('PLANNING', undefined), false, 'a manifest not read yet asks nothing');
});
