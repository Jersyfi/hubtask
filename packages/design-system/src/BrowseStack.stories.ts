// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import BrowseStackDemo from './_BrowseStackDemo.svelte';

export default {
  title: 'Wave 5 · Shell/BrowseStack',
  component: BrowseStackDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'motion', 'zoom'],
} satisfies StoryMeta;

export const browse: Story = {
  name: 'Browse',
  about:
    'The workspace level as a stack on compact (§9): groups as cards of 44 px rows. Press IT operations to go a level down; ‹ or an edge swipe goes up, and the change is announced.',
  args: { mode: 'browse' },
};

export const hub: Story = {
  name: 'A hub',
  about:
    'The three parts as on the desktop; a page with children has a separate › to descend.',
  args: { mode: 'hub' },
};

export const queues: Story = {
  name: 'Queues',
  about:
    'A stack step of its own with live counts, the starred one first.',
  args: { mode: 'queues' },
};

export const reorder: Story = {
  name: 'Reorder mode',
  about:
    'Grips at the end of each row, Done in the bar.',
  args: { mode: 'reorder' },
};
