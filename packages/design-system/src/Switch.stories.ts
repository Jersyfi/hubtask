// SPDX-License-Identifier: BUSL-1.1
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
