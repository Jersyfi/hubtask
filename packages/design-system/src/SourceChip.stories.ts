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
  name: 'The four sources',
  about:
    'Set here, a default, a plan, the installation - one chip beside the name for every value somebody may have set above the reader (P-06, P-07). Press one: the value, its source and whom to ask (UC-WRK-32 check 5). A lock is a mark and a word.',
};

export const beside: Story = {
  name: 'Beside a locked setting',
  about: 'The chip says who set it at the name; the switch itself is unavailable, and says why as a bubble at the switch.',
  args: { mode: 'beside' },
};
