// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import TableDemo from './_TableDemo.svelte';

export default {
  title: 'Wave 2 · Lists/Table',
  component: TableDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'density', 'zoom', 'width'],
} satisfies StoryMeta;

export const entries: Story = {
  name: 'A real table',
  about:
    'A grid of divs looks identical and is a different thing to anybody not looking at it: no row and column relationships, no “column 3 of 7” as focus moves, no way to read down a column. So this is a `<table>` with `<th scope="col">`, and the association is the element’s rather than aria attributes reimplementing it. The caption is its accessible name; a column of controls hides its heading on screen and keeps it in the tree. The first and last columns sit flush with the container, so the table’s text lines up with the prose above it instead of starting in a gutter.',
};

export const sorted: Story = {
  name: 'Sorted by a column',
  about:
    'A sortable heading is a button filling the cell — not the word alone, because “Fällig” is six characters wide — and the `<th>` carries `aria-sort`, which is what a screen reader reads. Press one column three times: ascending, descending, and **back to the order the caller handed over**, which is the press Atlassian’s table does not have and the reason a list with a meaningful default order can get it back. The unsorted mark is drawn quietly rather than hidden until hover, because a column whose sortability only appears under a pointer is one a touch reader never finds.',
  args: { mode: 'sorted' },
};

export const paged: Story = {
  name: 'With pages under it',
  about:
    'Ninety-one entries the client already holds, twelve at a time. The range sentence is the component and the numbers are an addition to it: “13–24 of 91” answers the question a reader actually has, and is true at every width. Pull Width down past 600 px and the numbers drop out while the sentence and the two arrows stay — seven targets across a phone is seven targets nobody can hit. The ellipsis is a control: it is announced as the pages it stands for and goes to the middle of them.',
  args: { mode: 'paged' },
};

export const states: Story = {
  name: 'Busy, and a row marked',
  about:
    'A page on its way fades the rows and leaves them where they are, so the reader keeps their place and the page keeps its height — a spinner in the body’s place moves everything under it twice. The marked row carries a bar as well as a tint, because a row told apart by colour alone is a row half the readers cannot tell apart (rule 3).',
  args: { mode: 'states' },
};

export const empty: Story = {
  name: 'Empty, with its headings still on screen',
  about:
    'The emptiness is a row rather than a replacement for the table. Swapping the whole table for an `EmptyState` takes the column names off the screen and moves everything under it; here the reader can still see what the table would have held. The caller renders it unconditionally and the stylesheet draws it only when it is the only row left.',
  args: { mode: 'empty' },
};

export const wide: Story = {
  name: 'Wider than its container, in German',
  about:
    'It scrolls inside its own box rather than widening the page — a wide table that widened the page would make everything else scroll sideways with it. Narrow the width axis. The edge fades in as soon as there is something past it, which is four background gradients and no listener at all, so the website gets it too. The scroll container is focusable, because a region that scrolls has to be reachable by keyboard (WCAG 2.1.1) or its last columns are unreachable to anybody who does not use a pointer.',
  args: { mode: 'wide' },
};
