// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import PlanSummaryDemo from './_PlanSummaryDemo.svelte';

export default {
  title: 'Wave 3 · Hubtask/PlanSummary',
  component: PlanSummaryDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom', 'width'],
} satisfies StoryMeta;

export const plan: Story = {
  name: 'An import, checked',
  about:
    'A plan in words before the act: one sentence a change, its kind a mark beside it, what would be refused said as a line of its own, and how far it reaches under the list. Never a key or a path.',
};

export const again: Story = {
  name: 'The same file again',
  about: 'Nothing new: the one line says everything is left as it is.',
  args: { mode: 'again' },
};

export const empty: Story = {
  name: 'Nothing to do',
  about: 'An empty plan says so in a sentence rather than drawing an empty list.',
  args: { mode: 'empty' },
};
