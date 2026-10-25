// SPDX-License-Identifier: Apache-2.0
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

export const inRow: Story = {
  name: 'In a row of buttons',
  about:
    'A note, unavailable. The reason is a bubble at the control on hover, focus and tap, and takes no space: the row with it and the row without it have the same shape. Trying it reaches neither its own handler nor the row.',
  args: { mode: 'row' },
};

export const inTableRow: Story = {
  name: 'In a table row',
  about: 'The same in a table row: the row keeps its height whether the control is available or not.',
  args: { mode: 'table' },
};

export const marked: Story = {
  name: 'Marked tokens, label read not drawn',
  about:
    'The capture line: what the field read stands marked behind the words as they are typed, and the label is read but not drawn because the dialog around it says what the field is for. Typing on keeps the marks under their words.',
  args: { mode: 'marked' },
};
