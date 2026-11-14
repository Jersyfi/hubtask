// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import Demo from './_FloatingBarDemo.svelte';

export default {
  title: 'Wave 7 · Work surface/FloatingBar',
  component: Demo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom', 'width'],
} satisfies StoryMeta;

export const all: Story = {
  name: 'One raised strip',
  about: 'The surface a page\'s writing bar, a board\'s islands and its selection bar share: one radius, one surface, one shadow. Where it stands is its user\'s.',
};
