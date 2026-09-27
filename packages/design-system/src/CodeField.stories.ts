// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import CodeFieldDemo from './_CodeFieldDemo.svelte';

export default {
  title: 'Wave 1 · Forms/CodeField',
  component: CodeFieldDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom', 'width', 'motion'],
} satisfies StoryMeta;

export const resting: Story = {
  name: 'Six places, one field',
  about:
    'Type into it, then paste into it, then walk back with Backspace: all three work, because there is one native input and the places are a picture of it. Switch Motion to Reduced and the caret stops blinking without the layout moving.',
};

export const invalid: Story = {
  name: 'Refused',
  about:
    'Rule 3: the message is the statement and the red underline is its echo. Nothing here is carried by colour alone.',
  args: { mode: 'invalid' },
};

export const recovery: Story = {
  name: 'Eight places, grouped in fours',
  about:
    'The contract allows six to eight characters, and a recovery code is the long one. The group is where the eye breaks the number, and it is a prop rather than a second component.',
  args: { mode: 'recovery' },
};
