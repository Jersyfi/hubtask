// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import BadgeDemo from './_BadgeDemo.svelte';

export default {
  title: 'Wave 1 · Feedback/Badge',
  component: BadgeDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom'],
} satisfies StoryMeta;

export const tones: Story = {
  name: 'The five tones',
  about:
    'Rule 3 made visible: every tone that means something carries its mark as well as its colour. Read the row in greyscale — a screenshot in a document, a printout, or a reader with a colour vision deficiency — and it still says which is which.',
};

export const german: Story = {
  name: 'The same row in German',
  about:
    'Rule 4: nothing here is a fixed width, so a badge grows with its word and wraps rather than clipping it. Pull Width down to compact on top of this.',
  args: { isLong: true },
};
