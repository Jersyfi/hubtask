// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import WidgetDemo from './_WidgetDemo.svelte';

export default {
  title: 'Wave 7 · Work surface/Widget',
  component: WidgetDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom', 'width'],
} satisfies StoryMeta;

export const today: Story = {
  name: 'Today’s cards (scene today)',
  about: 'One row of equal cards in a `WidgetStrip`: tile, title (whole words), one act as a button, the figure, one line. A figure that warns carries `figureTone`: its colour, a glyph and a word. On a phone the strip swipes. A card whose role the reader lacks is absent, not empty.',
};

export const overview: Story = {
  name: 'Tiles of the overview (scene hub)',
  about: 'The same anatomy with content instead of a figure; a quiet count at the head’s end.',
  args: { mode: 'overview' },
};

export const busy: Story = {
  name: 'Loading',
  about: 'The content’s place held by lines, so nothing below moves when it arrives; the region is `aria-busy`.',
  args: { mode: 'busy' },
};
