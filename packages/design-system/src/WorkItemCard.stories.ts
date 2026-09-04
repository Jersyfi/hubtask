// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import BoardDemo from './_BoardDemo.svelte';

export default {
  title: 'Wave 3 · Domain/WorkItemCard',
  component: BoardDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'density', 'zoom', 'width'],
} satisfies StoryMeta;

export const covers: Story = {
  name: 'With a cover, and without',
  about:
    '§4 asks for `cover` as a colour **or** an image — the same field with two shapes (domain-model.md §3.4), and a card that supported one would render half the entries wrong rather than plainly. A colour cover is a strip rather than a filled card: a label token’s background was measured against its own foreground, not against the card’s, so filling the card would put the title on a tint nobody checked it against.',
  args: { mode: 'covers' },
};

export const board: Story = {
  name: 'On a board, with labels',
  about:
    'The card is a **link**, for the reason ListRow is: readers open cards in a new tab constantly, and a div with a click handler takes that away. A completed card is struck through as well as dimmed — rule 3, so it reads in greyscale.',
};
