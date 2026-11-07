// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { test } from 'node:test';

import { EMOJI_GROUPS, MAX_MATCHES, pickable, searchEmojis } from './emojis.ts';

const require = createRequire(import.meta.url);
const set = (language: string) => pickable(JSON.parse(readFileSync(require.resolve(`emojibase-data/${language}/compact.json`), 'utf8')));

// The picker offers emojibase's set in the reader's language (fourth answers, 2): every group it
// shows has emoji, the skin-tone components are not offered on their own, and the set is in order.
test("the picker's set has every group it shows, in emojibase's order, without the components", () => {
  const all = set('en');
  for (const group of EMOJI_GROUPS) assert.ok(all.some((each) => each.group === group.order), group.key);
  assert.ok(all.every((each) => each.group !== 2));
  assert.ok(all.every((each, at) => at === 0 || (all[at - 1]?.order ?? 0) <= each.order));
  assert.equal(all[0]?.unicode, '😀');
});

test('a search finds by label and tag in the reader’s language, the label first, bounded', () => {
  const german = set('de');
  assert.equal(searchEmojis(german, 'grinsendes', 'de')[0]?.unicode, '😀');
  assert.ok(searchEmojis(german, 'lol', 'de').some((each) => each.unicode === '😀'));
  const hearts = searchEmojis(set('en'), 'heart', 'en');
  assert.ok(hearts.length > 0 && hearts.length <= MAX_MATCHES);
  assert.ok(hearts[0]?.label.includes('heart'));
  assert.deepEqual(searchEmojis(german, '   ', 'de'), []);
  assert.deepEqual(searchEmojis(german, 'qqqxxyz', 'de'), []);
});

test('an entry without characters, a label or a shown group is never offered', () => {
  assert.deepEqual(pickable([{ label: 'x', group: 0 }, { unicode: '😀', group: 0 }, { unicode: '🏻', label: 'light skin tone', group: 2 }, { unicode: 'x', label: 'none' }]), []);
});
