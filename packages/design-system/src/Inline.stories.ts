// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import InlineDemo from './_InlineDemo.svelte';

export default {
  title: 'Wave 0 · Primitives/Inline',
  component: InlineDemo,
  status: 'draft',
  // Rule 4 lives here more than anywhere else in wave 0: a row of three buttons is the layout that
  // fits in English and not in German, which is why `wrap` defaults to on.
  axes: ['text', 'dir', 'width', 'zoom'],
} satisfies StoryMeta;

export const resting: Story = {
  name: 'Three items in a row',
  about:
    'The default: wrapping is on. Pull the Width axis down to compact and the row becomes two lines rather than an overflow.',
};

export const longLabels: Story = {
  name: 'The same row in German',
  about:
    'Set Text to +40 % on top of this and rule 4 stops being a prediction. The row wraps; nothing is cut off and nothing scrolls sideways.',
  args: { isLong: true },
};

export const noWrap: Story = {
  name: 'Wrapping switched off',
  about:
    'What `isWrapping={false}` costs, shown deliberately. At compact width with the pseudo-locale on, this is the layout rule 4 is about — which is why the prop defaults the other way and needs a reason.',
  args: { isLong: true, isWrapping: false },
};

export const spaceBetween: Story = {
  name: 'Pushed to both ends',
  about:
    'With Direction: Both, `justify: between` mirrors. The first item sits at the start of the writing direction, not at the left of the screen.',
  args: { justify: 'between' },
};
