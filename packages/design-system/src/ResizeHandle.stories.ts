// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import ResizeHandleDemo from './_ResizeHandleDemo.svelte';

export default {
  title: 'Wave 5 · Shell/ResizeHandle',
  component: ResizeHandleDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'motion', 'zoom', 'width'],
} satisfies StoryMeta;

export const end: Story = {
  name: 'On the end of a column',
  about:
    'The navigation column’s edge. Drag it, or focus it and press the arrows; Home and End go to the bounds and a double-click puts the default back. A `separator` with a value, so a screen reader hears the width. The line shows where the pointer or the focus is, and while dragging.',
};

export const start: Story = {
  name: 'On the start of a panel',
  about:
    'The entry panel’s edge: dragging away from the panel widens it on either side and in either reading direction, so in a right-to-left document the arrows still point where the edge goes.',
  args: { edge: 'inline-start' },
};
