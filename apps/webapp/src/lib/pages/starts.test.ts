// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { isBlankBody, OWN_CHIPS, startChange, startChips, startKey } from './starts.ts';

test('the chips are Hubtask\'s templates, the most used first, then the first of the space\'s own', () => {
  const own = [{ id: 'a', title: 'Runbook', icon: '📗' }, { id: 'b', title: 'Postmortem', icon: null }, { id: 'c', title: 'Third', icon: null }];
  const chips = startChips(own);
  assert.deepEqual(chips.slice(0, 5).map(startKey), ['shipped:meeting', 'shipped:decision', 'shipped:retro', 'shipped:howto', 'shipped:brief']);
  assert.deepEqual(chips.slice(5).map(startKey), own.slice(0, OWN_CHIPS).map((row) => `own:${row.id}`));
  assert.equal(startChips([]).length, 5);
});

test('a page offers its starts while its body is blank', () => {
  assert.equal(isBlankBody(undefined), true);
  assert.equal(isBlankBody('\n\n  \n'), true);
  assert.equal(isBlankBody('Hallo'), false);
});

test('a start gives its title and mark only where the page has none of its own', () => {
  const start = { title: 'Decision record', body: '## Context\n', icon: '⚖️' };
  assert.deepEqual(startChange({ title: 'Untitled', icon: null }, 'Untitled', start), { body: '## Context\n', title: 'Decision record', icon: '⚖️' });
  assert.deepEqual(startChange({ title: '  ', icon: null }, 'Untitled', start), { body: '## Context\n', title: 'Decision record', icon: '⚖️' });
  assert.deepEqual(startChange({ title: 'Pricing', icon: '💶' }, 'Untitled', start), { body: '## Context\n' });
});
