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

export const narrow: Story = {
  name: 'At the narrowest sidebar',
  about:
    'The sidebar dragged to its minimum: a long figure - the running sprint’s name and days - is cut with an ellipsis first, and the row’s name keeps its room; a count beside a long name stays whole and the name is cut instead. A figure never grows past its token.',
  args: { mode: 'narrow' },
};

export const rail: Story = {
  name: 'Folded to the rail',
  about:
    'The rail: one mark per row, centred; the name is the accessible name and the tooltip.',
  args: { mode: 'rail' },
};

export const privateHubs: Story = {
  name: 'Private hubs, one opened in an emergency',
  about:
    'A private hub carries the lock after its name. For thirty days after the workspace’s owner opened one through the emergency access the shield takes the lock’s place, and its name says both facts - while the hour runs on the warning surface (an extra: the shape says it), afterwards as quiet as the lock. One badge per row.',
  args: { mode: 'private' },
};

export const ownMark: Story = {
  name: 'A mark the caller draws',
  about:
    'A row whose mark is data rather than an icon: the current workspace’s own symbol in the phone’s account sheet, in the mark’s box and as decorative as every mark. The row’s name names it; the selection surface and aria-current say it is the current one.',
  args: { mode: 'mark' },
};
