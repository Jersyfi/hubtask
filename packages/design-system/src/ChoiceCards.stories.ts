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
