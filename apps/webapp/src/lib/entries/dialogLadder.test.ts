// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The collections' dialogs on the page system's ladder (page-system §5.1): each hands its one
// primary, named with its verb, to the dialog - the only place a phone's full-screen dialog shows
// it, in its bar - and keeps *Cancel* beside it in the foot; no primary or danger button is left in
// its body. Read from the source, so every dialog of the family is held, opened or not.

import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';

const src = new URL('../../', import.meta.url);
const read = (path: string) => readFileSync(new URL(path, src), 'utf8');

/** The dialog in a file whose tag names this title code: its tag, and its body to its end. */
function dialogOf(source: string, titleCode: string): { tag: string; body: string } {
  for (let start = source.indexOf('<Dialog'); start >= 0; start = source.indexOf('<Dialog', start + 1)) {
    let depth = 0;
    let tagEnd = start;
    for (let i = start; i < source.length; i += 1) {
      const ch = source[i];
      if (ch === '{') depth += 1;
      else if (ch === '}') depth -= 1;
      else if (ch === '>' && depth === 0) { tagEnd = i; break; }
    }
    const tag = source.slice(start, tagEnd);
    if (tag.includes(`'${titleCode}'`)) return { tag, body: source.slice(tagEnd, source.indexOf('</Dialog>', tagEnd)) };
  }
  assert.fail(`no dialog titled ${titleCode}`);
}

const LADDER: readonly (readonly [string, string])[] = [
  ['views/SprintView.svelte', 'app.sprints.close_title'],
  ['views/ContainerView.svelte', 'app.workspace.trash_confirm_title'],
  ['views/ContainerView.svelte', 'app.workspace.shared_confirm_title'],
  ['lib/entries/Board.svelte', 'app.board.delete_column'],
  ['lib/entries/ExportDialog.svelte', 'app.export.title'],
  ['lib/entries/FeedsDialog.svelte', 'app.feeds.revoke_title'],
  ['lib/entries/EntryList.svelte', 'app.entries.change_amount'],
  ['lib/entries/BucketDialog.svelte', 'app.board.edit_column'],
  ['lib/workspace/PositionDialog.svelte', 'app.place.position_title'],
  ['views/HubDeskView.svelte', 'app.hub_desk.save_as_queue'],
  ['views/HubDeskView.svelte', 'app.hub_desk.delete_title'],
  ['views/TrashView.svelte', 'app.trash.purge_title'],
  ['views/TrashView.svelte', 'app.trash.empty_title'],
  ['lib/entries/ViewsPanel.svelte', 'app.views.save_title'],
  ['lib/entries/ViewsPanel.svelte', 'app.views.delete_title'],
  ['lib/entries/FeedsDialog.svelte', 'app.feeds.title'],
  ['lib/workspace/CollectionMoveDialog.svelte', 'app.move.title'],
];

for (const [file, title] of LADDER) {
  test(`${file} · ${title}: the primary is the dialog's, none in its body`, () => {
    const { tag, body } = dialogOf(read(file), title);
    assert.match(tag, /\bprimary=\{[^]*?\{ ?label: /u, 'the primary is not handed to the dialog');
    assert.doesNotMatch(body, /<Button\b[^>]*tone="(primary|danger)"/u, 'a primary stays in the body');
    assert.doesNotMatch(body, /<Button\b(?![^>]*tone=)(?![^>]*size="sm")[^>]*>/u, 'a button without a tone - a primary - stays in the body');
  });
}

test('lib/entries/TemplatesDialog.svelte: each step hands its primary to the dialog, none in its body', () => {
  const source = read('lib/entries/TemplatesDialog.svelte');
  const start = source.indexOf('<PanelOrDialog');
  const tagEnd = source.indexOf('\n>', start);
  const tag = source.slice(start, tagEnd);
  for (const verb of ['app.templates.go', 'app.templates.generate', 'app.templates.save']) assert.match(tag, new RegExp(`label: t\\('${verb}'\\)`, 'u'), `${verb} is not the dialog's primary`);
  const body = source.slice(tagEnd, source.indexOf('</PanelOrDialog>'));
  assert.doesNotMatch(body, /<Button\b(?![^>]*tone=)(?![^>]*size="sm")[^>]*>/u, 'a primary stays in the body');
});

test('lib/workspace/CollectionMoveDialog.svelte: a Dialog M on every width, no sheet with its own buttons (§5.1)', () => {
  assert.doesNotMatch(read('lib/workspace/CollectionMoveDialog.svelte'), /<Sheet\b/u);
});

test('lib/entries/EntryList.svelte: a row\'s labels are a popover, a bottom sheet on a phone; a category is a choice, no primary', () => {
  const source = read('lib/entries/EntryList.svelte');
  assert.match(source, /viewport\.isCompact\}\s*<IconButton icon="tag"[^>]*onclick=\{\(\) => \(labelling = row\.item\)\}/u, 'the phone opens no sheet for labels');
  assert.match(source, /<Sheet title=\{t\('app\.labels\.on_entry'/u);
  const { body } = dialogOf(source, 'app.entries.choose_category');
  assert.doesNotMatch(body, /tone=\{[^}]*'primary'/u, 'the chosen category is told by a primary');
});
