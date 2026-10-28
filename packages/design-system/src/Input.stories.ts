// SPDX-License-Identifier: Apache-2.0
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

export const inRow: Story = {
  name: 'In a row of buttons',
  about:
    'A title field, unavailable. The reason is a bubble at the control on hover, focus and tap, and takes no space: the row with it and the row without it have the same shape. Trying it reaches neither its own handler nor the row.',
  args: { mode: 'row' },
};

export const inTableRow: Story = {
  name: 'In a table row',
  about: 'The same in a table row: the row keeps its height whether the control is available or not.',
  args: { mode: 'table' },
};

export const range: Story = {
  name: 'A range, unavailable',
  about:
    '`readonly` does not reach a range, so the guard refuses the press and every key but leaving: the arrows move nothing, and the reason shows.',
  args: { mode: 'range' },
};

export const code: Story = {
  name: 'A value a machine reads',
  about:
    'A host or a short name is typed as it will be read, so it is drawn in the mono face: `face="code"`. The placeholder is in the same face, which is what makes it read as the value that stands in.',
  args: { mode: 'code' },
};
