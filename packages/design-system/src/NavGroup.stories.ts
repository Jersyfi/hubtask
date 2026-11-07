// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import NavSidebarDemo from './_NavSidebarDemo.svelte';

export default {
  title: 'Wave 5 · Shell/NavGroup',
  component: NavSidebarDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'motion', 'zoom'],
} satisfies StoryMeta;

export const tree: Story = {
  name: 'The content tree, dragged',
  about:
    'Contents as one tree of pages and whiteboards (§2.4): sticky head with its count, ＋ and ⋯; 15 top-level rows, then n more; the filter from 30 entries with All · Pages · Whiteboards. Drag a row by the row: 4 px before it starts, the line at the target level, a page outlined for inside, a whiteboard refuses with its sentence on the drag image; a folded page opens after 600 ms; Escape cancels; the drop lands, is highlighted, and Undo is offered. Alt+arrows on a focused row order and indent.',
  args: { mode: 'drag' },
};

export const queues: Story = {
  name: 'Queues and a small tree',
  about:
    'An IT hub: queues under their tool row with live counts, the starred one first and urgent; the content tree with an open branch.',
  args: { mode: 'queue' },
};

export const empty: Story = {
  name: 'An empty part',
  about:
    'A personal hub: only lists, and the quiet Create the first page for somebody who may create.',
  args: { mode: 'empty' },
};
