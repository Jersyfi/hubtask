// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import RecordHeadDemo from './_RecordHeadDemo.svelte';

export default {
  title: 'Wave 6 · Compositions/RecordHead',
  component: RecordHeadDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom'],
} satisfies StoryMeta;

export const group: Story = {
  name: 'A group',
  about:
    'The way back to the list, the kind as the eyebrow, the name, one line of facts, *Edit* and ⋯ with the destructive act last.',
};
