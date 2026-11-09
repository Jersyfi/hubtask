// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { MACHINE_TABS, machinesTabPath } from './machinespage.ts';

test('the service accounts are the screen\'s own address and the apps its tab', () => {
  assert.deepEqual(MACHINE_TABS.map((tab) => tab.id), ['service-accounts', 'apps']);
  assert.equal(machinesTabPath('service-accounts'), '/administration/machines');
  assert.equal(machinesTabPath('apps'), '/administration/machines?tab=apps');
  assert.equal(machinesTabPath('unknown'), '/administration/machines');
});
