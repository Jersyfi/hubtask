// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { offModuleOf } from './moduleroute.ts';

test('a module page whose module is off says so before it reads anything', () => {
  assert.equal(offModuleOf('service-desk', ['WORKFLOWS']), 'SERVICE_DESK');
  assert.equal(offModuleOf('service-desk', ['SERVICE_DESK']), undefined);
  assert.equal(offModuleOf('workflows', []), 'WORKFLOWS');
  assert.equal(offModuleOf('service-desk', undefined), undefined, 'a manifest not read yet withholds nothing');
  assert.equal(offModuleOf('webhooks', []), undefined, 'a page of no module');
});
