// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import type { Page } from '@hubtask/sync-engine';

import { SHIPPED_SPACES, SHIPPED_TEMPLATES, ancestorsOf, homePageOf, publicPreviewOf, writersHere, draftStateOf, hasSubpages, shippedSpace, writtenBody, caretOf, changedFrom, withLabels, exportFileName, publicLinkAddress, pageTree, shippedTemplate, subtreeOf, verifiedUntil } from './pages.ts';

const page = (id: string, parent: string | null, position: string, title = id): Page => ({
  id, hub_id: 'hub', parent_id: parent, title, icon: null, position, template: false, content_language: null, owner_id: null,
  verified_until: null, verified_at: null, verified_by: null, due_for_review: false, archived: false, restricted: false, daily_for: null, trashed_at: null,
  created_by: 'a', created_at: '', updated_by: 'a', updated_at: '', version: 1,
});

const rows = [page('b', null, 'b'), page('a', null, 'a'), page('a2', 'a', 'b'), page('a1', 'a', 'a'), page('a1x', 'a1', 'a'), page('orphan', 'gone', 'c')];

test('the tree orders siblings by position and keeps a page whose parent is out of reach (UC-DOC-01 check 6)', () => {
  const tree = pageTree(rows);
  assert.deepEqual(tree.map((node) => node.page.id), ['a', 'b', 'orphan']);
  assert.deepEqual(tree[0]?.children.map((node) => node.page.id), ['a1', 'a2']);
  assert.equal(tree[0]?.children[0]?.children[0]?.depth, 2);
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

test("Hubtask's space templates write several pages at once in the reader's words (UC-DOC-05 check 6)", () => {
  const words = (key: string) => `«${key}»`;
  for (const key of SHIPPED_SPACES) {
    const space = shippedSpace(key, words);
    assert.equal(space.title, `«app.pages.space.${key}.title»`);
    assert.ok((space.subpages ?? []).length >= 2, key);
    for (const sub of space.subpages ?? []) assert.ok(sub.title.startsWith('«app.pages.') && sub.body?.length, key);
  }
});

test("a space's own template offers its subpages only when it has any (UC-DOC-05 check 5)", () => {
  assert.equal(hasSubpages(rows, 'a'), true);
  assert.equal(hasSubpages(rows, 'b'), false);
});

test("a writer edits a space's draft; a page tells whether it waits to be published (UC-DOC-15)", () => {
  const base = { ...page('p', null, 'a'), body: 'v1' };
  assert.equal(writtenBody(base), 'v1');
  assert.equal(draftStateOf(base), 'live');
  const drafted = { ...base, drafts: true, draft: { title: 'p', body: 'v2', updated_by: 'a', updated_at: '' } };
  assert.equal(writtenBody(drafted), 'v2');
  assert.equal(draftStateOf(drafted), 'draft');
  assert.equal(draftStateOf({ ...base, drafts: true, unpublished: true }), 'unpublished');
  assert.equal(draftStateOf({ ...base, drafts: true, draft: null }), 'published');
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

test('a public link opens the page on this server, its token the whole credential (UC-DOC-08 check 4)', () => {
  assert.equal(publicLinkAddress('https://hub.example.org/', 'hbt_page_abc'), 'https://hub.example.org/api/v1/public/pages/hbt_page_abc');
});

test('a typed label is drawn in the written form the server keeps (UC-DOC-01 check 9)', () => {
  assert.deepEqual(withLabels(['hr'], ' How-To, HR  policy '), ['how-to', 'hr', 'policy']);
  assert.deepEqual(withLabels([], '   '), []);
});

test('a search narrowed by when a page changed starts that many days back, or not at all', () => {
  const now = new Date('2026-10-31T12:00:00Z');
  assert.equal(changedFrom('any', now), undefined);
  assert.equal(changedFrom('week', now), '2026-10-24T12:00:00.000Z');
});

test('a space opens on its first top page, never on a template (user-experience.md §7.1)', () => {
  assert.equal(homePageOf(rows), 'a');
  assert.equal(homePageOf([{ ...page('t', null, 'a'), template: true }, page('z', null, 'z')]), 'z');
  assert.equal(homePageOf([page('child', 'gone', 'a')]), 'child');
  assert.equal(homePageOf([]), undefined);
});

test('the public preview counts what the link shows and what it leaves out (UC-DOC-08 check 5)', () => {
  const id = (n: number) => `00000000-0000-0000-0000-${String(n).padStart(12, '0')}`;
  const body = `See [](hubtask:page/${id(1)}) and [](hubtask:entry/${id(2)}) twice [](hubtask:entry/${id(2)}).\n\n`
    + `![](hubtask:media/${id(3)})\n\n![](hubtask:view/${id(4)})\n\n![](hubtask:media/${id(5)})\n`;
  assert.deepEqual(publicPreviewOf(body), { media: 2, pages: 1, others: 2 });
  assert.deepEqual(publicPreviewOf('plain words'), { media: 0, pages: 0, others: 0 });
});

test('who else writes on a page is named once, never the reader (user-experience.md §7.1)', () => {
  const states = new Map<number, unknown>([[1, { user: { name: 'Me' } }], [2, { user: { name: 'Bert' } }], [3, { user: { name: 'Bert' } }], [4, {}], [5, { user: { name: ' ' } }]]);
  assert.deepEqual(writersHere(states, 1), ['Bert']);
});
