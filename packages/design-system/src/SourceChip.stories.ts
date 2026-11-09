// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import SourceChipDemo from './_SourceChipDemo.svelte';

export default {
  title: 'Wave 3 · Hubtask/SourceChip',
  component: SourceChipDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom', 'width'],
} satisfies StoryMeta;

export const sources: Story = {
  name: 'Every source',
  about:
    'Set here (the accent dot), a default, a level above, a lock by the plan or the installation - quiet words at the end of the name\'s line for every value somebody may have set above the reader (P-06, P-07), no frame and no fill. Press one: the value, its source and whom to ask (UC-WRK-32 check 5). A lock is a mark and a word. Look at it with Theme set to Both.',
};

export const beside: Story = {
  name: 'Beside a locked setting',
  about: 'The words say who locked it at the name; the switch itself is unavailable, and says why as a bubble at the switch.',
  args: { mode: 'beside' },
};
