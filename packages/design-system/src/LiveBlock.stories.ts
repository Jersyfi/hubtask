// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import LiveBlockDemo from './_LiveBlockDemo.svelte';

export default {
  title: 'Wave 7 · Work surface/LiveBlock',
  component: LiveBlockDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom'],
} satisfies StoryMeta;

export const view: Story = {
  name: 'A view in a page',
  about: 'The product tile, what it is, "Live" in words and the way to the original over the live rows (S-4).',
  args: { mode: 'view' },
};

export const board: Story = {
  name: 'A board in a page',
  about: 'A whiteboard or one frame of it under the same head.',
  args: { mode: 'board' },
};
