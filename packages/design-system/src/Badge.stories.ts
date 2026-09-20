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

export const bold: Story = {
  name: 'Subtle above, bold below',
  about:
    'A status as a surface (ADR-0061): the subtle form is the tone’s surface, border and text — the form for a list, where every row has one. The bold form is the tone’s accent under inverse text, for the one badge on a screen that has to be seen first: a failed run, a lost connection. A screen with three bold badges has misread the rule. Both rows are measured by the contrast test in both modes.',
  args: { hasBold: true },
};

export const german: Story = {
  name: 'The same row in German',
  about:
    'Rule 4: nothing here is a fixed width, so a badge grows with its word and wraps rather than clipping it. Pull Width down to compact on top of this.',
  args: { isLong: true },
};
