// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import Demo from './_SummaryStripDemo.svelte';

export default {
  title: 'Wave 7 · Work surface/SummaryStrip',
  component: Demo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom', 'width'],
} satisfies StoryMeta;

export const all: Story = {
  name: 'Four figures, two rows, two on a phone',
  about: 'A product home\'s key figures under its band: four in a row from `expanded`, two rows of two below it, and on a phone the two the home names.',
};
