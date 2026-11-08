// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import SheetDemo from './_SheetDemo.svelte';

export default {
  title: 'Wave 5 · Shell/Sheet',
  component: SheetDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'motion', 'zoom', 'width'],
} satisfies StoryMeta;

export const context: Story = {
  name: 'The context sheet',
  about:
    'A long press on a row (§9): Star, Create under, Move to…, Change order, Archive, Move to the trash. A heading and Close; Escape or the scrim closes it.',
  args: { mode: 'context' },
};

export const create: Story = {
  name: 'New on Contents',
  about:
    'Page, Page from a template…, Whiteboard; Collection where allowed.',
  args: { mode: 'new' },
};
