// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import type { Page } from '@hubtask/sync-engine';

import { SHIPPED_TEMPLATES, ancestorsOf, caretOf, exportFileName, pageTree, shippedTemplate, subtreeOf, verifiedUntil, visibleRows } from './pages.ts';

const page = (id: string, parent: string | null, position: string, title = id): Page => ({
  id, hub_id: 'hub', parent_id: parent, title, icon: null, position, template: false, content_language: null, owner_id: null,
  verified_until: null, verified_at: null, verified_by: null, due_for_review: false, archived: false, trashed_at: null,
  created_by: 'a', created_at: '', updated_by: 'a', updated_at: '', version: 1,
});

const rows = [page('b', null, 'b'), page('a', null, 'a'), page('a2', 'a', 'b'), page('a1', 'a', 'a'), page('a1x', 'a1', 'a'), page('orphan', 'gone', 'c')];

test('the tree orders siblings by position and keeps a page whose parent is out of reach (UC-DOC-01 check 6)', () => {
  const tree = pageTree(rows);
  assert.deepEqual(tree.map((node) => node.page.id), ['a', 'b', 'orphan']);
  assert.deepEqual(tree[0]?.children.map((node) => node.page.id), ['a1', 'a2']);
  assert.equal(tree[0]?.children[0]?.children[0]?.depth, 2);
});

test('only an opened branch draws its pages', () => {
  const tree = pageTree(rows);
  assert.deepEqual(visibleRows(tree, new Set()).map((node) => node.page.id), ['a', 'b', 'orphan']);
  assert.deepEqual(visibleRows(tree, new Set(['a', 'a1'])).map((node) => node.page.id), ['a', 'a1', 'a1x', 'a2', 'b', 'orphan']);
});

test('the pages above one are opened to show it; a move never lands under its own (UC-DOC-01 check 2)', () => {
  assert.deepEqual(ancestorsOf(rows, 'a1x'), ['a', 'a1']);
  assert.deepEqual([...subtreeOf(rows, 'a')].sort(), ['a', 'a1', 'a1x', 'a2']);
});

test('Hubtask\'s templates are written in the reader\'s words (UC-DOC-05 check 4)', () => {
  const words = (code: string) => `«${code}»`;
  for (const key of SHIPPED_TEMPLATES) {
    const { title, body } = shippedTemplate(key, words);
    assert.equal(title, `«app.pages.template.${key}.title»`);
    assert.match(body, new RegExp(`^«app\\.pages\\.template\\.${key}\\.lead»`));
    assert.ok(body.includes('## «app.pages.template.'), key);
  }
  assert.ok(shippedTemplate('meeting', words).body.includes('- [ ]'));
});

test('a verification runs whole days from now; a caret colour is the same for a person everywhere', () => {
  assert.equal(verifiedUntil(new Date('2026-10-29T10:00:00Z'), 30), '2026-11-28T10:00:00.000Z');
  assert.equal(caretOf('person-1'), caretOf('person-1'));
  assert.ok(caretOf('person-2') >= 0 && caretOf('person-2') < 8);
  assert.equal(exportFileName('Handbook/Onboarding.md'), 'Handbook-Onboarding.md');
});

test('an exported DOCX document is saved as its decoded bytes, text as its UTF-8 (UC-DOC-09 check 4)', async () => {
  const { exportBytes } = await import('./pages.ts');
  assert.deepEqual([...exportBytes({ content: 'UEs=', encoding: 'BASE64' })], [0x50, 0x4b]);
  assert.deepEqual([...exportBytes({ content: 'ä', encoding: 'TEXT' })], [0xc3, 0xa4]);
});
