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
    'Every node a leaf. The mark column is what aligns them: it is first and at one inline position for every level, so a flat list lines up with a tree beside it and the twist column that is not needed here takes no room at the start.',
  args: { mode: 'flat' },
};

export const long: Story = {
  name: 'The same tree in German',
  about:
    'Rule 4 against the component with the least room: the indent takes width from the label at every level, not from the mark in front of it. The labels truncate; the indent does not collapse, because the depth is the information.',
  args: { mode: 'long' },
};

export const rail: Story = {
  name: 'Folded to its marks',
  about:
    'The fold is a drawing, not a narrower panel: one mark per row, centred in the column the layout token gives it, with no twist, no label and no indent. The label stays the row’s accessible name and becomes its tooltip. Depth is the one thing a rail cannot draw, so it does not try — it lists the roots, and a branch pressed there opens its subtree in a flyout beside the column. A column that was merely narrowed put the mark past its own edge, which is what issue 915 was.',
  args: { isRail: true },
};

export const flyout: Story = {
  name: 'A branch opened from the rail',
  about:
    'The flyout is this same component with the branch as its root, so the keyboard walk, the announcements and the current mark are the tree’s own and there is nothing second to keep in step. It is positioned and dismissed by the code every overlay here uses: beside the row, Escape closes it, a press outside closes it, and focus returns to the mark it came from. Towards the children — ArrowRight in a left-to-right document — opens it; towards the parent closes it.',
  args: { isRail: true, opened: 'work' },
};
