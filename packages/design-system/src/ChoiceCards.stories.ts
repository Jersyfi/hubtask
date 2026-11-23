// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import ChoiceCardsDemo from './_ChoiceCardsDemo.svelte';

export default {
  title: 'Wave 1 · Forms/ChoiceCards',
  component: ChoiceCardsDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom'],
} satisfies StoryMeta;

export const question: Story = {
  name: 'A choice that needs a sentence',
  about:
    'Each answer is a card: its title and one line of what choosing it means. The whole card is pressed; the arrow keys walk and choose as in any radio group. Nothing is chosen until the person chooses.',
};

export const marked: Story = {
  name: 'Cards with a mark',
  about:
    'An optional mark between the dot and the words, for a choice among starts or kinds. The chosen card is marked by its edge, its surface and the filled dot, never by colour alone.',
  args: { mode: 'marked' },
};

export const gated: Story = {
  name: 'One card that cannot be chosen',
  about: 'A card may carry its own reason, said under its line and announced with the option.',
  args: { mode: 'gated' },
};

export const tiles: Story = {
  name: 'Tiles of a gallery that is a choice',
  about:
    'The start of a hub: `layout="grid"` fills the width with tiles no narrower than a gallery card, one per row on a phone; `mark="tick"` draws an empty ring in the top end corner that fills with the tick when chosen. The families are fieldsets with their legend inside the one radio group: the arrow keys walk and choose across them, and Tab enters at the chosen tile. A summary is cut after two lines; the products are pills.',
  args: { mode: 'tiles' },
};

export const blank: Story = {
  name: 'The start from nothing, chosen',
  about:
    'The empty hub is dashed until it is chosen, so a choice that sets up nothing reads as one; chosen, its edge is solid and doubled like any other. Choose a template to see it dashed.',
  args: { mode: 'blank' },
};

export const refusedTile: Story = {
  name: 'One tile that cannot be chosen',
  about: 'A refused tile says its reason under its words and is announced with it; the arrow keys pass over it, and the reason moves nothing.',
  args: { mode: 'refused' },
};

export const german: Story = {
  name: 'German names of real length',
  about: 'A name that wraps onto a second line and a summary cut after two: the tiles of a family grow together, the tick stays in its corner.',
  args: { mode: 'german' },
};

export const headed: Story = {
  name: 'Tiles under a heading of their own',
  about:
    'Where a heading above a search and a filter already asks the question, `isLabelHidden` keeps the legend as the group’s name for a reader and draws it nowhere; the first family’s heading then opens the tiles.',
  args: { mode: 'headed' },
};
