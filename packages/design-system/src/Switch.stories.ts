// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import SwitchDemo from './_SwitchDemo.svelte';

export default {
  title: 'Wave 1 · Forms/Switch',
  component: SwitchDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'motion', 'zoom'],
} satisfies StoryMeta;

export const resting: Story = {
  name: 'A setting, not a value',
  about:
    'A checkbox is a value in a form that is submitted; a switch is a setting that applies the moment it moves. Switch Direction to Both — the knob travels towards the end of the writing direction, not towards the right of the screen.',
};

export const reducedMotion: Story = {
  name: 'The same, without the travel',
  about:
    'Set Motion to Reduced. Rule 3 is why this still reads: the knob is in a different place, so the state is legible with no animation and no colour at all.',
};

export const unavailable: Story = {
  name: 'Switched off, with the reason',
  about: 'A setting that cannot be changed says why it cannot be.',
  args: { mode: 'unavailable' },
};

export const inRow: Story = {
  name: 'In a row of buttons',
  about:
    'A setting your workspace decides. The reason is a bubble at the control on hover, focus and tap, and takes no space: the row with it and the row without it have the same shape. Trying it reaches neither its own handler nor the row.',
  args: { mode: 'row' },
};

export const inTableRow: Story = {
  name: 'In a table row',
  about: 'The same in a table row: the row keeps its height whether the control is available or not.',
  args: { mode: 'table' },
};

export const busy: Story = {
  name: 'Busy, in place',
  about: 'Switch busy and back: the spinner turns inside the knob, the switch keeps its size, and pressing it does nothing while it works.',
  args: { mode: 'busy' },
};

export const saving: Story = {
  name: 'Saving a setting',
  about:
    'A switch applies at once: while it saves, the progress is inside the switch and nothing appears beside it; afterwards a short confirmation offers *Undo* (UC-WRK-32 check 6). The confirmation is the caller’s `Toast`.',
  args: { mode: 'saving' },
};
