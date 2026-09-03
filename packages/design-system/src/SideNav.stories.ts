// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import SideNavDemo from './_SideNavDemo.svelte';

export default {
  title: 'Wave 2 · Structure/SideNav',
  component: SideNavDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'density', 'zoom', 'width'],
} satisfies StoryMeta;

export const tree: Story = {
  name: 'Hubs and their collections',
  about:
    'A tree is a keyboard interaction before it is a picture: one stop in the tab order, the arrows move between visible nodes, the direction arrows expand and collapse, Home and End reach the ends. It does not wrap, unlike a menu — a list has a shape, and running off the end of it loses the reader’s place in that shape. In RTL the arrow towards the children is the one pointing left, and the indent mirrors with it.',
};

export const flat: Story = {
  name: 'No branches at all',
  about:
    'Every node a leaf. The twist column stays, so the labels of a flat list line up with those of a tree beside it rather than shifting by an icon’s width.',
  args: { mode: 'flat' },
};

export const long: Story = {
  name: 'The same tree in German',
  about:
    'Rule 4 against the component with the least room: an indent takes width from the label at every level. The labels truncate; the indent does not collapse, because the depth is the information.',
  args: { mode: 'long' },
};
