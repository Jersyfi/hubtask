// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { labelTokens } from '@hubtask/design-system';
import { ICON_NAMES } from '@hubtask/design-system/icons';

import { SYMBOLS, chosenColour, chosenSymbol, lookBody, markOf } from './look.ts';

test('every symbol on offer is in the icon set, once', () => {
  assert.equal(new Set(SYMBOLS).size, SYMBOLS.length);
  for (const symbol of SYMBOLS) assert.ok((ICON_NAMES as readonly string[]).includes(symbol), symbol);
});

test('no symbol on offer is the mark of a level or of a container type (decision 14)', () => {
  for (const reserved of ['hub', 'collection', 'task', 'work-package', 'activity', 'container']) {
    assert.ok(!(SYMBOLS as readonly string[]).includes(reserved), reserved);
  }
});

test('the colour is read back only where it is one of the ten', () => {
  for (const token of labelTokens) assert.equal(chosenColour({ color_token: token }), token);
  assert.equal(chosenColour({ color_token: ' violet ' }), 'violet');
  for (const other of [undefined, null, '', 'raspberry', 'Violet']) {
    assert.equal(chosenColour({ color_token: other }), undefined, String(other));
  }
});

test('the symbol is read back only where it is one on offer', () => {
  assert.equal(chosenSymbol({ icon: 'house' }), 'house');
  for (const other of [undefined, null, '', 'trash', 'hub', 'not-an-icon']) {
    assert.equal(chosenSymbol({ icon: other }), undefined, String(other));
  }
});

test('a place nobody chose a symbol for keeps its type’s mark; the colour never decides it', () => {
  assert.equal(markOf({ type: 'HUB', icon: null }), 'hub');
  assert.equal(markOf({ type: 'COLLECTION', icon: 'trash' }), 'collection');
  assert.equal(markOf({ type: 'COLLECTION', icon: 'briefcase' }), 'briefcase');
  assert.equal(markOf({ type: 'HUB', icon: 'house' }), 'house');
});

test('none is sent as null, so that an edit clears the field rather than leaving it', () => {
  assert.deepEqual(lookBody(undefined, undefined), { icon: null, color_token: null });
  assert.deepEqual(lookBody('house', 'magenta'), { icon: 'house', color_token: 'magenta' });
});
