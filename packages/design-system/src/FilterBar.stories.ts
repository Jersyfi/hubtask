// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import FilterBarDemo from './_FilterBarDemo.svelte';

export default {
  title: 'Wave 6 · Compositions/FilterBar',
  component: FilterBarDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom'],
} satisfies StoryMeta;

export const people: Story = {
  name: 'People',
  about:
    'Search, filters as chips that each open their picker, *Clear* while one is active, the count at the end. Below `expanded` the first two chips and *More filters*; on a phone the search fills the line, *Filters (1)* opens a sheet with every picker, and the count stands under it.',
};

export const audit: Story = {
  name: 'Audit, the time range first',
  about:
    'A log’s filter bar: the time range is the first chip.',
  args: { mode: 'audit' },
};
