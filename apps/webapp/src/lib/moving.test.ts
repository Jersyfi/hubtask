// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { boardMoveBody, pageMoveBody, pickerPlaces, placementOf, reordered } from './moving.ts';

// Team › (Holiday rules, Retro board), Onboarding
const tree = [
  { id: 'team', label: 'Team', children: [{ id: 'holiday', label: 'Holiday rules' }, { id: 'retro', label: 'Retro board', canHoldChildren: false }] },
  { id: 'onboarding', label: 'Onboarding' },
];

test('UC-WRK-38 check 5: where a row stands now is where Undo puts it back', () => {
  assert.deepEqual(placementOf(tree, 'retro'), { parentId: 'team', afterId: 'holiday' });
  assert.deepEqual(placementOf(tree, 'holiday'), { parentId: 'team', afterId: null });
  assert.deepEqual(placementOf(tree, 'onboarding'), { parentId: null, afterId: 'team' });
  assert.equal(placementOf(tree, 'gone'), undefined);
});

test('UC-CNV-10 check 2: a placement becomes movePage\'s and moveWhiteboard\'s body', () => {
  assert.deepEqual(pageMoveBody({ parentId: null, afterId: null }), {});
  assert.deepEqual(pageMoveBody({ parentId: 'team', afterId: 'retro' }), { parent_id: 'team', after_id: 'retro' });
  assert.deepEqual(boardMoveBody({ parentId: null, afterId: 'team' }), { parent_page_id: null, after_id: 'team' });
});

test('UC-WRK-38 check 6: Move to… greys a whiteboard, the row itself and what is under it, with the reason', () => {
  const places = pickerPlaces(tree, 'team', 'Top of Contents');
  assert.deepEqual(places.map((each) => [each.id, each.refusal ?? null]), [
    [null, null], ['team', 'self'], ['holiday', 'self'], ['retro', 'self'], ['onboarding', null],
  ]);
  const forHoliday = pickerPlaces(tree, 'holiday', 'Top of Contents');
  assert.equal(forHoliday.find((each) => each.id === 'retro')?.refusal, 'no-children');
  // Into a page it lands after the page's last child; at the top after the last top row.
  assert.deepEqual(forHoliday.find((each) => each.id === 'team')?.placement, { parentId: 'team', afterId: 'retro' });
  assert.deepEqual(forHoliday[0]?.placement, { parentId: null, afterId: 'onboarding' });
});

test('UC-WRK-38 check 6: a place that nests too deep is refused', () => {
  const places = pickerPlaces(tree, 'onboarding', 'Top', 2);
  assert.equal(places.find((each) => each.id === 'holiday')?.refusal, 'too-deep');
  assert.equal(places.find((each) => each.id === 'team')?.refusal, undefined);
});

test('UC-WRK-37 check 6: a flat list ordered anew', () => {
  assert.deepEqual(reordered(['a', 'b', 'c'], 'c', null), ['c', 'a', 'b']);
  assert.deepEqual(reordered(['a', 'b', 'c'], 'a', 'b'), ['b', 'a', 'c']);
  assert.deepEqual(reordered(['a', 'b'], 'x', null), ['a', 'b']);
});
