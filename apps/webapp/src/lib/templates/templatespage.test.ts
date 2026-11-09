// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { figures, isInUse, narrowed, tabOf, tabPath } from './templatespage.ts';

const SCRUM = { key: 'scrum_team', origin: 'HUBTASK' as const, taken_version: 1, update_available: true };
const ITIL = { key: 'itil_service_desk', origin: 'HUBTASK' as const, taken_version: 2, update_available: false };
const HOUSE = { key: 'household', origin: 'HUBTASK' as const, taken_version: null, update_available: false };
const OURS = { key: 'ours', origin: 'WORKSPACE' as const, update_available: false };
const ALL = [SCRUM, ITIL, HOUSE, OURS];
const words = (each: { key: string }) => [each.key];

test('each tab has its address, the catalogue the screen\'s own; an unknown tab is the catalogue', () => {
  assert.equal(tabPath('catalogue'), '/administration/templates');
  assert.equal(tabPath('in-use'), '/administration/templates?tab=in-use');
  assert.equal(tabPath('presets'), '/administration/templates?tab=presets');
  assert.equal(tabOf(undefined), 'catalogue');
  assert.equal(tabOf('presets'), 'presets');
  assert.equal(tabOf('elsewhere'), 'catalogue');
});

test('a template is in use once the workspace took a version; the figures count from the list', () => {
  assert.equal(isInUse(SCRUM), true);
  assert.equal(isInUse(HOUSE), false);
  assert.equal(isInUse(OURS), false);
  assert.deepEqual(figures(ALL), { catalogue: 4, inUse: 2, updates: 1 });
});

test('the in-use tab, the family chip and the search narrow together', () => {
  assert.deepEqual(narrowed(ALL, { tab: 'catalogue', search: '', words }), ALL);
  assert.deepEqual(narrowed(ALL, { tab: 'in-use', search: '', words }), [SCRUM, ITIL]);
  assert.deepEqual(narrowed(ALL, { tab: 'catalogue', family: 'service', search: '', words }), [ITIL]);
  assert.deepEqual(narrowed(ALL, { tab: 'catalogue', family: 'workspace', search: '', words }), [OURS]);
  assert.deepEqual(narrowed(ALL, { tab: 'in-use', search: 'scrum', words }), [SCRUM]);
});
