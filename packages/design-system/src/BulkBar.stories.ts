// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import BulkBarDemo from './_BulkBarDemo.svelte';

export default {
  title: 'Wave 6 · Compositions/BulkBar',
  component: BulkBarDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom'],
} satisfies StoryMeta;

export const selecting: Story = {
  name: 'Three selected',
  about:
    'How many, up to three verbs, ⋯ for the rest, *Done*. Escape ends the selection.',
};
