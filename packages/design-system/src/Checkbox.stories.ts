// SPDX-License-Identifier: BUSL-1.1
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
