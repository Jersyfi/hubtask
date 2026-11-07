// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import NavSidebarDemo from './_NavSidebarDemo.svelte';

export default {
  title: 'Wave 5 · Shell/NavItem',
  component: NavSidebarDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'motion', 'zoom'],
} satisfies StoryMeta;

export const hub: Story = {
  name: 'A hub level’s rows',
  about:
    'Rows of the sidebar (navigation concept §2.6): a 30 px row, the twist’s room kept, a mark or a tile, the name, the figure - quiet in mono, or on the danger surface where it asks for action. Hover a row: ⋯ and ＋ take the figure’s place, a grip appears where ordering is allowed. Overview is current: the brand bar, the selection surface, semibold, aria-current.',
  args: { mode: 'hub' },
};

export const rail: Story = {
  name: 'Folded to the rail',
  about:
    'The rail: one mark per row, centred; the name is the accessible name and the tooltip.',
  args: { mode: 'rail' },
};
