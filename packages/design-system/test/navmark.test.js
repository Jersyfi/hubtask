// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A row's own mark in the navigation (design-system.md §4, NavItem): a snippet the caller draws in
// the mark's box in place of the icon - a workspace's own symbol on the current row of the phone's
// sheet. Checked without a browser as a reading of its source, the bargain `conventions.test.js`
// makes; the story is `ownMark`, and the walk that draws it is the webapp's `devicelist` suite.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const source = fs.readFileSync(path.resolve(here, '..', 'src', 'NavItem.svelte'), 'utf8');
const stories = fs.readFileSync(path.resolve(here, '..', 'src', 'NavItem.stories.ts'), 'utf8');

test('a row may draw its own mark, in the mark box, decorative, in place of the icon', () => {
  assert.match(source, /mark\?: Snippet;/, 'the snippet is a prop of its own');
  assert.match(source,
    /<span class="mark" aria-hidden="true">\s*\{#if unreadLabel\}<UnreadDot \/>\{:else if mark\}\{@render mark\(\)\}\{:else if icon && color\}/,
    'drawn in the mark box, which is hidden from the tree, after the unread dot and before the icon');
  assert.equal((source.match(/\{@render mark\(\)\}/g) ?? []).length, 1, 'drawn in one place only');
});

test('the own mark has a story', () => {
  assert.match(stories, /export const ownMark: Story = \{/);
});
