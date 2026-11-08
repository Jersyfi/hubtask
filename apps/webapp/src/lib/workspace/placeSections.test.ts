// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { SETTINGS } from './structureDraft.ts';
import { STRUCTURE_GROUPS, groupOf, orderSections, sectionOf } from './placeSections.ts';

const ids = (level: 'HUB' | 'COLLECTION', given: readonly string[]) => orderSections(level, given.map((id) => ({ id }))).map((each) => each.id);

test('a collection\'s sections stand in the one order of user-experience.md §5.7', () => {
  const given = ['general', 'labels', 'fields', 'templates', 'policies', 'people', 'template', 'types', 'workflow', 'views', 'rules'];
  assert.deepEqual(ids('COLLECTION', given), ['general', 'template', 'types', 'fields', 'labels', 'templates', 'workflow', 'views', 'rules', 'policies', 'people']);
});

test('a hub\'s sections: its products and template apart, the shared ones in the same order', () => {
  const given = ['general', 'modules', 'calendars', 'code', 'chat', 'people', 'template', 'types', 'workflow', 'views', 'rules'];
  assert.deepEqual(ids('HUB', given), ['general', 'modules', 'template', 'types', 'workflow', 'views', 'rules', 'calendars', 'code', 'chat', 'people']);
});

test('a section the place does not draw leaves no gap, and an unknown one goes last', () => {
  assert.deepEqual(ids('HUB', ['people', 'unknown', 'general', 'types']), ['general', 'types', 'people', 'unknown']);
});

test('every structure setting is drawn in exactly one section', () => {
  const placed = Object.values(STRUCTURE_GROUPS).flat();
  assert.deepEqual([...placed].sort(), [...SETTINGS].sort());
  assert.equal(new Set(placed).size, placed.length);
  assert.equal(groupOf('views'), 'views');
  assert.equal(groupOf('automation'), 'rules');
});

test('an old address opens where its content went', () => {
  assert.equal(sectionOf('structure'), 'types');
  assert.equal(sectionOf('automation'), 'rules');
  assert.equal(sectionOf('labels'), 'labels');
});
