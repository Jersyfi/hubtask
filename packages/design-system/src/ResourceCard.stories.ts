// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import ResourceCardDemo from './_ResourceCardDemo.svelte';

export default {
  title: 'Wave 6 · Compositions/ResourceCard',
  component: ResourceCardDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom'],
} satisfies StoryMeta;

export const cards: Story = {
  name: 'Mark, name, sentence, chips, state',
  about:
    'Level 1′: a hairline and no shadow; the hover a step of surface and a firmer border. A link, a dialog opener, and one that opens nothing.',
};

export const compact: Story = {
  name: 'On a phone',
  about:
    'Below medium the card reads in three lines: the mark beside the name, the sentence held to two lines, the chips and the state on one. Narrow the window, not the pane, to see it.',
};
