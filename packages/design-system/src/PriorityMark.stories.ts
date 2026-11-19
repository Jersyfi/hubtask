// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import Demo from './_PriorityMarkDemo.svelte';

export default {
  title: 'Wave 7 · Work surface/PriorityMark',
  component: Demo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom', 'width'],
} satisfies StoryMeta;

export const all: Story = {
  name: 'The four levels',
  about: 'One shape for a priority in a cell, a head and the rail: glyph and word in the level’s tone - up for P1 and P2, a dash for P3, down for P4.',
};
