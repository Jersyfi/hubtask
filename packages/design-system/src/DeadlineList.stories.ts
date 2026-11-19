// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import Demo from './_DeadlineListDemo.svelte';

export default {
  title: 'Wave 7 · Work surface/DeadlineList',
  component: Demo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom', 'width'],
} satisfies StoryMeta;

export const all: Story = {
  name: 'A ticket’s deadlines',
  about: 'ET-6 second round: one row per deadline, the most urgent first - name, whose it is, what is left in its tone with a glyph, a bar across, the due time under it -, the kept line and *All deadlines as a table*.',
};
