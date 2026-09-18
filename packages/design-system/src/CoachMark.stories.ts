// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import CoachMarkDemo from './_CoachMarkDemo.svelte';

export default {
  title: 'Wave 3 · Domain/CoachMark',
  component: CoachMarkDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom', 'width'],
} satisfies StoryMeta;

export const middle: Story = {
  name: 'A step in the middle',
  about:
    'The mark on its own, unanchored: the caption, the body, the count, and three verbs — next, back, skip. Resolved text throughout; the register is voice-and-tone.md’s, and no exclamation mark does the work. A `dialog` for focus purposes: focus lands on Next when it opens.',
};

export const first: Story = {
  name: 'The first step',
  about: 'No step comes before, so back is not offered rather than disabled: there is nothing to say about why.',
  args: { hasBack: false },
};
