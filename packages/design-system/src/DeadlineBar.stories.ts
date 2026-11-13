// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import Demo from './_DeadlineBarDemo.svelte';

export default {
  title: 'Wave 7 · Work surface/DeadlineBar',
  component: Demo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom', 'width'],
} satisfies StoryMeta;

export const all: Story = {
  name: 'Running, at risk, breached, paused, met',
  about: 'The bar with its words; a warning or a breach adds a glyph, a paused clock its own mark. With `due` the words and the due time stand under the bar, as in the rail.',
};
