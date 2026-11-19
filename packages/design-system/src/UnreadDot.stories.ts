// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import UnreadDotDemo from './_UnreadDotDemo.svelte';

export default {
  title: 'Wave 5 · Shell/UnreadDot',
  component: UnreadDotDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom'],
} satisfies StoryMeta;

export const posts: Story = {
  name: 'Beside a post’s title and in the hub column',
  about:
    'News not read yet (work-surface concept B-2): a dot in the accent, never the danger surface the urgent figure stands on. Beside a title it says "Not read yet" to a screen reader; in a row of the hub column it stands where the mark would, and the row’s name carries the words.',
};
