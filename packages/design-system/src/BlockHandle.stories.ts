// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import FloatingToolbarDemo from './_FloatingToolbarDemo.svelte';

export default {
  title: 'Wave 7 · Work surface/BlockHandle',
  component: FloatingToolbarDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom'],
} satisfies StoryMeta;

export const line: Story = {
  name: 'At a line',
  about: '"+" inserts a block below the line; the handle opens a menu of what can be done with it, so nothing is only by dragging (S-1).',
  args: { mode: 'line' },
};
