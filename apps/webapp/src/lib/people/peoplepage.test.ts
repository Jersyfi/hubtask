// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { PEOPLE_TABS, tabPath } from './peoplepage.ts';

test('the people are at the screen\'s own address, the other tabs by their name', () => {
  assert.equal(tabPath('people'), '/administration/people');
  assert.deepEqual(PEOPLE_TABS.slice(1).map((tab) => tabPath(tab.id)), [
    '/administration/people?tab=groups',
    '/administration/people?tab=roles',
    '/administration/people?tab=permissions',
    '/administration/people?tab=settings',
  ]);
});
