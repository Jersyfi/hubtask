// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import DataTableDemo from './_DataTableDemo.svelte';

export default {
  title: 'Wave 6 · Compositions/DataTable',
  component: DataTableDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'density', 'zoom'],
} satisfies StoryMeta;

export const full: Story = {
  name: 'People, nine rows',
  about:
    'One region on the canvas: the head sticks under the app bar while the page scrolls, the row is a link to the person’s record, ⋯ holds the verbs with the destructive one last, the foot says what is shown and keeps the rows per page. Narrow the window: below `expanded` the priority-3 columns (Groups, Last seen) give way; on a phone each row is a list row — the name, the e-mail under it, the standing and ⋯ at the end, ⋯ opening a sheet.',
};

export const selecting: Story = {
  name: 'Selection mode',
  about: '*Select* puts a checkbox before each row, and the `BulkBar` stands where the head was: the count, up to three verbs, ⋯, *Done*. Escape ends it.',
  args: { mode: 'selecting' },
};

export const busy: Story = {
  name: 'Rows on their way',
  about: 'The rows fade where they are; nothing moves and the reader keeps their place.',
  args: { mode: 'busy' },
};

export const empty: Story = {
  name: 'Nobody yet',
  about: 'The empty state of the *unused* kind inside the region, under the headings, with the act that fills it.',
  args: { mode: 'empty' },
};

export const filtered: Story = {
  name: 'Filtered to nothing',
  about: 'The *filtered* kind, with *Clear filters*.',
  args: { mode: 'filtered' },
};

export const log: Story = {
  name: 'Grouped by day',
  about: 'A log’s rows: a sunken group row where the day changes, words instead of keys, a row that opens its record in a dialog, and `LoadMore` at the foot, saying once what arrived.',
  args: { mode: 'log' },
};

export const tree: Story = {
  name: 'A tree, its folds before the titles',
  about:
    'A hierarchy drawn as a table: each row’s fold stands before its title in `lead`, outside the press that opens the row, so the fold is its own target and never a button within a link.',
  args: { mode: 'tree' },
};
