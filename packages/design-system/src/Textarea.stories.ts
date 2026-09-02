// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import TextareaDemo from './_TextareaDemo.svelte';

export default {
  title: 'Wave 1 · Forms/Textarea',
  component: TextareaDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom', 'width'],
} satisfies StoryMeta;

export const resting: Story = {
  name: 'Three lines, growing',
  about:
    'It grows with its content rather than scrolling inside a fixed box — rule 4 says everything grows by 40 %, and a note written in German in a box sized for English is a note read through a slot.',
};

export const invalid: Story = {
  name: 'With an error',
  about: 'As Input: the message is the state and the border is the echo.',
  args: { mode: 'invalid' },
};
