// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import CheckboxGroupDemo from './_CheckboxGroupDemo.svelte';

export default {
  title: 'Wave 1 · Forms/CheckboxGroup',
  component: CheckboxGroupDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom'],
} satisfies StoryMeta;

export const resting: Story = {
  name: 'Several of a few, under one question',
  about:
    'A fieldset with its legend: entering the first box, a screen reader names the question before the option. Untick everything and press *Create token*: nothing was switched off beforehand.',
};

export const missing: Story = {
  name: 'Nothing chosen, said once for the group',
  about:
    'The form was pressed with nothing ticked (design-system.md §4, case 3). The group is marked `data-invalid` and described by the one sentence; every box is `aria-invalid`, so focus moved to the first box arrives on a control that says what is wrong. Tick one and the sentence goes.',
  args: { mode: 'missing' },
};
