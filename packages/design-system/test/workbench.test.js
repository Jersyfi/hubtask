// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The workbench's index (F9-03): components in waves, the filter, and the current component.
// Pure functions over the loaded groups, so that what the sidebar shows is decided here rather
// than in a browser.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { componentOf, filtered, matches, waves } from '../workbench/lib/index.ts';

const meta = (title) => ({ title, component: () => {}, status: 'draft', axes: [] });
const group = (title, stories) => {
  const [wave, name] = title.split('/');
  const m = meta(title);
  return {
    group: wave,
    title: name,
    meta: m,
    stories: stories.map((s, i) => ({ id: `${title.toLowerCase()}--${i}`, name: s, meta: m })),
  };
};

const GROUPS = [
  group('Wave 1 · Forms/Button', ['Tones', 'With an icon']),
  group('Wave 1 · Forms/Input', ['Resting', 'Invalid']),
  group('Wave 3 · Domain/TaskRow', ['The three levels, expanded', 'The same rows in German']),
  group('Wave 1 · Feedback/Badge', ['The five tones']),
];

test('waves keep the loaded order and hold their components', () => {
  const out = waves(GROUPS);
  assert.deepEqual(
    out.map((w) => [w.wave, w.components.map((c) => c.title)]),
    [
      ['Wave 1 · Forms', ['Button', 'Input']],
      ['Wave 3 · Domain', ['TaskRow']],
      ['Wave 1 · Feedback', ['Badge']],
    ],
  );
});

test('a query matches a component by its name, its wave, or a story it holds', () => {
  const [button, , taskRow] = GROUPS;
  assert.ok(matches(button, 'butt'));
  assert.ok(matches(button, 'BUTTON'), 'case does not matter');
  assert.ok(matches(button, 'forms'), 'the wave is searchable');
  assert.ok(matches(taskRow, 'three levels'), 'a story name finds its component');
  assert.ok(!matches(button, 'three levels'));
  assert.ok(matches(button, ''), 'an empty query matches everything');
  assert.ok(matches(button, '   '), 'so does whitespace');
});

test('filtering drops the components a query does not keep, and the waves left empty', () => {
  const out = filtered(GROUPS, 'in');
  // "in" is in Input, in "The three levels, expanded" (TaskRow) and nowhere in Button or Badge.
  assert.deepEqual(
    out.map((w) => [w.wave, w.components.map((c) => c.title)]),
    [
      ['Wave 1 · Forms', ['Input']],
      ['Wave 3 · Domain', ['TaskRow']],
    ],
  );
  assert.deepEqual(filtered(GROUPS, 'nothing here'), []);
  assert.equal(filtered(GROUPS, '').length, 3, 'no query keeps every wave');
});

test('the current component is the one holding the story, or nothing', () => {
  assert.equal(componentOf(GROUPS, GROUPS[2].stories[1].id)?.title, 'TaskRow');
  assert.equal(componentOf(GROUPS, 'no-such-story'), undefined);
  assert.equal(componentOf(GROUPS, null), undefined);
});
