// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import BlockListDemo from './_BlockListDemo.svelte';

export default {
  title: 'Wave 6 · Compositions/BlockList',
  component: BlockListDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom'],
} satisfies StoryMeta;

export const workflow: Story = {
  name: 'Statuses and steps',
  about:
    'One line per block: grip, name or status badge, what it does, ⋯. Pressing a block opens its dialog. Reorder by the grip, by `Alt`+↑/↓ on a focused block, or by ⋯ *Move up*/*Move down*.',
};
