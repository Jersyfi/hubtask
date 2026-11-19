// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import Demo from './_WidgetStripDemo.svelte';

export default {
  title: 'Wave 7 · Work surface/WidgetStrip',
  component: Demo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom', 'width'],
} satisfies StoryMeta;

export const all: Story = {
  name: 'Equal cards, a strip on the phone',
  about: 'However many cards, they share the row equally; below `medium` the row scrolls sideways and each card snaps into place.',
};
