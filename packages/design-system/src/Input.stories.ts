// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import InputDemo from './_InputDemo.svelte';

export default {
  title: 'Wave 1 · Forms/Input',
  component: InputDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom', 'width'],
} satisfies StoryMeta;

export const resting: Story = {
  name: 'Label, hint, and an icon inside',
  about:
    'Switch Direction to Both: the label, the icon and the caret all move to the other end together, because nothing here says left or right. The focus ring is on the shell, so the icon is inside it.',
};

export const invalid: Story = {
  name: 'With an error',
  about:
    'Rule 3: colour never stands alone. The message is text and the thicker border is only its echo — in greyscale the field is still visibly the one that is wrong.',
  args: { mode: 'invalid' },
};

export const unavailable: Story = {
  name: 'Switched off, with the reason',
  about: 'The reason is rendered under the field and pointed at by `aria-describedby`.',
  args: { mode: 'unavailable' },
};
