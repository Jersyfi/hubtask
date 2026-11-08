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
    'Recent as a flyout row, Starred, Products with their tiles and n more, Hubs with › and their order. The scrollbar is thin and shows only on hover, its track then drawn with a hairline at its inner edge; its room is reserved.',
  args: { mode: 'workspace' },
};

export const hub: Story = {
  name: 'A hub level',
  about:
    'The head: All hubs (focus lands there after a level change), the hub’s tile and name, the quick switch, ☆ and ⋯; then Overview and the tools above a hairline, Collections, Contents. A change of level slides in, none under reduced motion.',
  args: { mode: 'hub' },
};

export const narrow: Story = {
  name: 'The head at the narrowest sidebar',
  about:
    'At the minimum width the hub’s name wraps to two lines at most and ends with an ellipsis; the quick switch, ☆ and ⋯ keep their places and the name never runs under them.',
  args: { mode: 'narrow' },
};

export const wiki: Story = {
  name: 'A page current in the tree',
  about:
    'The current page’s branch is open and its row carries the bar.',
  args: { mode: 'wiki' },
};
