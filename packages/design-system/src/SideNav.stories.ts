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

export const bands: Story = {
  name: 'Bands, captioned and not',
  about:
    'A band is a mark on the first node of a group, so the tree stays one list with one keyboard walk. What draws it is an element of its own between the rows — the hairline, the air above it, and the caption where the group has a name; the foot of a navigation has none, and the separation is what says it is a different kind of thing. Nothing about a band reaches a row: every row in the column is the same height, whichever band it opens. Drawing the separation as padding on the row is what made the first row of each band short and its background sit against its text (issue 1010).',
  args: { mode: 'bands' },
};

export const railBands: Story = {
  name: 'Bands, folded',
  about:
    'Folded, a band is the hairline alone. A caption is five words in a column 56 px wide, which is words in a place with no room for any (issue 1012) — so the words wait for the fold to open, exactly as a row’s label does, and what the reader keeps is the grouping.',
  args: { mode: 'bands', isRail: true },
};

export const branch: Story = {
  name: 'A branch is a place too',
  about:
    'A hub is a screen before it is a container — its settings, its collections, the control that makes another one — so pressing its row goes there and opens it, and the twist at the end of the row is what closes it again. The twist is a control with a name and no tab stop of its own: the tree keeps one stop, the arrows keep expanding, and `aria-expanded` on the row stays the one statement about the state. A caller that names no twist keeps the old behaviour, where the row folds and nothing navigates (issue 1022).',
  args: { mode: 'tree' },
};
