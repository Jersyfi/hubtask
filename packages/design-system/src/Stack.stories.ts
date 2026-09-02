// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import StackDemo from './_StackDemo.svelte';

export default {
  title: 'Wave 0 · Primitives/Stack',
  component: StackDemo,
  status: 'draft',
  // `text` and `width` are the two that carry rule 4: a stack of German labels is where a column
  // first runs out of room. `dir` because `align: start` has to change ends by itself.
  axes: ['text', 'dir', 'width', 'zoom'],
} satisfies StoryMeta;

export const resting: Story = {
  name: 'One gap, three children',
  about:
    'The default arrangement. Nothing here is a margin on a child — switch to the +40 % pseudo-locale and the spacing between the items stays exactly what it was, because the gap belongs to the stack.',
};

export const alignStart: Story = {
  name: 'Aligned to start, which is not left',
  about:
    'With Direction: Both, the two panes mirror. A component that had written `align-items: flex-start` as `left` would show identical panes here, which is the failure this axis exists to catch.',
  args: { align: 'start' },
};

export const longLabels: Story = {
  name: 'Labels that already wrap',
  about:
    'German before the pseudo-locale touches it. Narrow the Width axis and the items wrap inside their own box; the gaps do not collapse and nothing overlaps.',
  args: { long: true, align: 'start' },
};

export const tightest: Story = {
  name: 'The smallest step',
  about: 'Gap 025 — two pixels. The scale has to be usable at both ends or callers invent values.',
  args: { gap: '025' },
};
