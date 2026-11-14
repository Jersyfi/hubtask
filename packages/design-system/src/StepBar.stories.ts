// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import StepBarDemo from './_StepBarDemo.svelte';

export default {
  title: 'Wave 7 · Work surface/StepBar',
  component: StepBarDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom', 'width'],
} satisfies StoryMeta;

export const stage: Story = {
  name: 'An object’s stage as a path',
  about: 'IN-2: the steps before the current one done with a check, the current one filled (`aria-current="step"`), the later ones quiet; glyph and fill say it, never the colour alone. Right to left, every arrow is mirrored. Below `medium` the bar scrolls sideways with the current step in view.',
};

export const ends: Story = {
  name: 'At the first and the last step',
  about: 'Nothing done yet, and everything done: the first step has no notch, the last no point.',
  args: { mode: 'ends' },
};
