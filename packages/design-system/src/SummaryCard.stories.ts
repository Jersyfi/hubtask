// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import SummaryCardDemo from './_SummaryCardDemo.svelte';

export default {
  title: 'Wave 6 · Compositions/SummaryCard',
  component: SummaryCardDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom'],
} satisfies StoryMeta;

export const overview: Story = {
  name: 'Three tiles',
  about:
    'Regions, not raised cards: a title, a figure or a state with its mark, one line, one link.',
};
