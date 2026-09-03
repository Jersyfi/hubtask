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
    'A grid of divs looks identical and is a different thing to anybody not looking at it: no row and column relationships, no “column 3 of 7” as focus moves, no way to read down a column. So this is a `<table>` with `<th scope="col">`, and the association is the element’s rather than aria attributes reimplementing it. The caption is its accessible name; a column of controls hides its heading on screen and keeps it in the tree.',
};

export const wide: Story = {
  name: 'Wider than its container, in German',
  about:
    'It scrolls inside its own box rather than widening the page — a wide table that widened the page would make everything else scroll sideways with it. Narrow the width axis. The scroll container is focusable, because a region that scrolls has to be reachable by keyboard (WCAG 2.1.1) or its last columns are unreachable to anybody who does not use a pointer.',
  args: { mode: 'wide' },
};
