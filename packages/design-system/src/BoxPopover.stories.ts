// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import FloatingToolbarDemo from './_FloatingToolbarDemo.svelte';

export default {
  title: 'Wave 7 · Work surface/BoxPopover',
  component: FloatingToolbarDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom'],
} satisfies StoryMeta;

export const panel: Story = {
  name: 'Under a line',
  about: 'What "/" or "+" offers, opened under the line it was asked at, as an `InsertList`: the arrows choose, Enter inserts, a letter jumps, Escape or a press outside the text closes it (S-1).',
  args: { mode: 'panel' },
};
