// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import CheckboxDemo from './_CheckboxDemo.svelte';

export default {
  title: 'Wave 1 · Forms/Checkbox',
  component: CheckboxDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'motion', 'zoom'],
} satisfies StoryMeta;

export const threeStates: Story = {
  name: 'On, off, and partly',
  about:
    'The third state is the one a parent row has when some of its children are done. The input is transparent and on top rather than hidden, so the keyboard and the accessibility tree still have it — walk the tab order and the ring appears on the painted box.',
};

export const longLabels: Story = {
  name: 'Labels that wrap',
  about:
    'Set Text to +40 %. The box stays aligned with the first line of the label rather than being dragged down by the block.',
  args: { isLong: true },
};

export const unavailable: Story = {
  name: 'Switched off, with the reason',
  about: 'The reason sits under the label, indented past the box, and is pointed at by `aria-describedby`.',
  args: { mode: 'unavailable' },
};

export const inRow: Story = {
  name: 'In a row of buttons',
  about:
    'A box with its label drawn in the row, and with its label hidden in the table row - the case that used to hang a reason under nothing. The reason is a bubble at the control on hover, focus and tap, and takes no space: the row with it and the row without it have the same shape. Trying it reaches neither its own handler nor the row.',
  args: { mode: 'row' },
};

export const inTableRow: Story = {
  name: 'In a table row',
  about: 'The same in a table row: the row keeps its height whether the control is available or not.',
  args: { mode: 'table' },
};

export const busy: Story = {
  name: 'Busy, in place',
  about: 'Switch busy and back: the spinner turns inside the box, the box keeps its size, and pressing it does nothing while it works.',
  args: { mode: 'busy' },
};
