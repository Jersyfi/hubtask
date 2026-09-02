// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import IconDemo from './_IconDemo.svelte';

export default {
  title: 'Wave 1 · Foundations/Icon',
  component: IconDemo,
  status: 'draft',
  // `theme` because `currentColor` is the whole contract and a mark that named a colour would show
  // it here; `zoom` because the 24 and 16 px sizes are the pair SC 1.4.4 is met through page zoom
  // with; `dir` because a mark that means "next" must not point the same way in both directions.
  axes: ['theme', 'zoom', 'dir', 'width'],
} satisfies StoryMeta;

export const sheet: Story = {
  name: 'The whole set at 24 px',
  about:
    'Every declared icon, base and ours, on the grid they are drawn on. Switch Theme to Both: nothing may change but the colour, because no mark names one.',
};

export const small: Story = {
  name: 'The same set at 16 px',
  about:
    'The stroke is put back as the box shrinks — 1.5 × 24/16 — so the drawn weight reads the same at both sizes. Set Zoom to 200 % and compare: what changes is the size, not the weight.',
  args: { size: 'sm' },
};

export const domainMarks: Story = {
  name: 'The domain marks alone',
  about:
    'The nouns a general set cannot say: the three levels of the hierarchy sharing one box, the jumble that promises no order, the capability that is a switch. Where Lucide already says a noun well — a label is a tag, a comment a message-square — nothing is drawn here.',
  args: { mode: 'custom' },
};

export const insideText: Story = {
  name: 'Inside a sentence',
  about:
    'An icon takes the colour and the size of the text it sits in. Three colours of text, one icon component, and no token named anywhere in it.',
  args: { mode: 'text' },
};
