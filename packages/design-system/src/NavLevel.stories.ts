// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import NavSidebarDemo from './_NavSidebarDemo.svelte';

export default {
  title: 'Wave 5 · Shell/NavLevel',
  component: NavSidebarDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'motion', 'zoom'],
} satisfies StoryMeta;

export const workspace: Story = {
  name: 'The workspace level',
  about:
    'Recent as a flyout row, Starred, Products with their tiles and n more, Hubs with › and their order. The scrollbar is thin, has no track and shows only on hover; its room is reserved.',
  args: { mode: 'workspace' },
};

export const hub: Story = {
  name: 'A hub level',
  about:
    'The head: All hubs (focus lands there after a level change), the hub’s tile and name, the quick switch, ☆ and ⋯; then Overview and the tools above a hairline, Collections, Contents. A change of level slides in, none under reduced motion.',
  args: { mode: 'hub' },
};

export const wiki: Story = {
  name: 'A page current in the tree',
  about:
    'The current page’s branch is open and its row carries the bar.',
  args: { mode: 'wiki' },
};
