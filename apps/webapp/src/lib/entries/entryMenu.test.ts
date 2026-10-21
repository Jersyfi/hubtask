// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The entry menu as literal lists, per role, state, type and surface (UC-WRK-28 check 1). Literal on
// purpose: a test that compared the menu with what the gates it uses answer would agree with any
// mistake in them.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { entryMenu, idsOf, type EntryMenuContext, type EntrySurface } from './entryMenu.ts';

const t = (code: string, params?: Record<string, string | number>) =>
  params ? `${code}(${Object.values(params).join(',')})` : code;

const CARRIES: Record<string, readonly string[]> = {
  // The fixture's manifest (apps/webapp/e2e/fixture.mjs), as a real server answers it.
  TASK: ['COMPLETION', 'DUE_DATE', 'REMINDER', 'ASSIGNMENT', 'MEMBERS', 'BUCKET', 'NOTES', 'LABELS'],
  ACTIVITY: ['COMPLETION', 'DUE_DATE', 'REMINDER', 'ASSIGNMENT', 'HISTORY'],
};

function context(overrides: Partial<EntryMenuContext> & { type?: string } = {}): EntryMenuContext {
  const type = overrides.type ?? 'TASK';
  return {
    surface: 'list',
    item: { title: 'Buy milk', isCompleted: false, hasDue: false },
    archival: 'active',
    isCollectionReadOnly: false,
    mayChange: true,
    mayCreate: true,
    mayShare: false,
    carries: (capability) => (CARRIES[type] ?? []).includes(capability),
    level: { index: 1, count: 3, aboveTitle: 'Bread' },
    ...overrides,
  };
}

const WRITER = [
  'select', 'open', 'complete', 'dates', 'person', 'labels',
  'up', 'down', 'top', 'bottom', 'inside', 'out', 'elsewhere',
  'duplicate', 'copy-link', 'archive', 'trash',
];

test('OWNER and ADMIN: every action, Share among them', () => {
  assert.deepEqual(idsOf(entryMenu(context({ mayShare: true }), t)), [
    'select', 'open', 'complete', 'dates', 'person', 'labels',
    'up', 'down', 'top', 'bottom', 'inside', 'out', 'elsewhere',
    'duplicate', 'share', 'copy-link', 'archive', 'trash',
  ]);
});

test('MEMBER, and a CONTRIBUTOR on their own entry: the same without Share', () => {
  assert.deepEqual(idsOf(entryMenu(context(), t)), WRITER);
});

test('a CONTRIBUTOR on somebody else\'s entry: nothing that writes it, not even off', () => {
  const items = entryMenu(context({ mayChange: false }), t);
  assert.deepEqual(idsOf(items), ['open', 'duplicate', 'copy-link']);
  assert.ok(items.every((item) => item.disabledReason === undefined));
});

test('VIEWER and GUEST: open and copy the link', () => {
  assert.deepEqual(idsOf(entryMenu(context({ mayChange: false, mayCreate: false }), t)), ['open', 'copy-link']);
});

test('while the role is being read, nothing that writes and no reason', () => {
  const items = entryMenu(context({ mayChange: undefined, mayShare: true }), t);
  assert.deepEqual(idsOf(items), ['open', 'copy-link']);
});

test('an entry archived in its own right: listed, off with the reason, except the way back and the trash', () => {
  const items = entryMenu(context({ archival: 'archived' }), t);
  assert.deepEqual(idsOf(items), WRITER.map((id) => (id === 'archive' ? 'unarchive' : id)));
  const on = items.filter((item) => item.disabledReason === undefined).map((item) => item.id);
  assert.deepEqual(on, ['select', 'open', 'duplicate', 'copy-link', 'unarchive', 'trash']);
  assert.ok(items.filter((item) => item.disabledReason).every((item) => item.disabledReason === 'app.entries.archived'));
});

test('a read-only area lists only what does not write, with no reason on the row', () => {
  for (const area of [{ archival: 'inherited' as const }, { isCollectionReadOnly: true }]) {
    const items = entryMenu(context(area), t);
    const expected = 'isCollectionReadOnly' in area ? ['open', 'copy-link'] : ['open', 'copy-link', 'duplicate'];
    assert.deepEqual(idsOf(items), expected);
    assert.ok(items.every((item) => item.disabledReason === undefined));
  }
});

test('what the type does not carry is not listed', () => {
  const ids = idsOf(entryMenu(context({ type: 'ACTIVITY' }), t));
  assert.ok(!ids.includes('labels'), 'an activity carries no labels');
  assert.ok(ids.includes('dates') && ids.includes('person') && ids.includes('complete'));
});

test('a completed entry offers to reopen it, a dated one to change the date', () => {
  const ids = idsOf(entryMenu(context({ item: { title: 'Buy milk', isCompleted: true, hasDue: true } }), t));
  assert.ok(ids.includes('reopen') && !ids.includes('complete'));
  const dates = entryMenu(context({ item: { title: 'x', isCompleted: false, hasDue: true } }), t).find((item) => item.id === 'dates');
  assert.equal(dates?.label, 'app.due.edit');
});

test('the edges of the level and the moves say why', () => {
  const first = entryMenu(context({ level: { index: 0, count: 2, insideReason: 'nothing above', outReason: 'already top' } }), t);
  const reason = (id: string) => first.find((item) => item.id === id)?.disabledReason;
  assert.equal(reason('up'), 'app.rank.already_first');
  assert.equal(reason('top'), 'app.rank.already_first');
  assert.equal(reason('down'), undefined);
  assert.equal(reason('inside'), 'nothing above');
  assert.equal(reason('out'), 'already top');
  const inside = first.find((item) => item.id === 'inside');
  assert.equal(inside?.label, 'app.move.inside_above');
  const named = entryMenu(context(), t).find((item) => item.id === 'inside');
  assert.equal(named?.label, 'app.move.inside(Bread)');
});

test('a level not read yet keeps the moves off until it arrives', () => {
  const items = entryMenu(context({ surface: 'search', level: undefined }), t);
  for (const id of ['up', 'down', 'top', 'bottom', 'inside', 'out']) {
    assert.equal(items.find((item) => item.id === id)?.disabledReason, 'app.move.moving', id);
  }
  assert.equal(items.find((item) => item.id === 'elsewhere')?.disabledReason, undefined);
});

test('the same menu on every surface; Select only where a bulk acts, the columns only on the board', () => {
  const surfaces: EntrySurface[] = ['list', 'subtree', 'board', 'timeline', 'search', 'today'];
  const columns = [{ id: 'column:doing', label: 'Move to Doing' }];
  const menus = Object.fromEntries(
    surfaces.map((surface) => [
      surface,
      idsOf(entryMenu(context({ surface, ...(surface === 'board' ? { extras: columns } : {}) }), t)),
    ]),
  );
  const common = (ids: string[]) => ids.filter((id) => id !== 'select' && !id.startsWith('column:'));
  for (const surface of surfaces) assert.deepEqual(common(menus[surface] ?? []), common(WRITER), surface);
  assert.deepEqual(surfaces.filter((surface) => menus[surface]?.[0] === 'select'), ['list', 'board']);
  assert.deepEqual((menus.board ?? []).filter((id) => id.startsWith('column:')), ['column:doing']);
  assert.equal(menus.board?.indexOf('column:doing'), (menus.board?.indexOf('elsewhere') ?? 0) + 1);
});

test('the trash is last, alone and destructive', () => {
  const items = entryMenu(context(), t);
  const last = items.at(-1);
  assert.equal(last?.id, 'trash');
  assert.equal(last?.isDestructive, true);
  assert.equal(last?.hasSeparatorBefore, true);
});
