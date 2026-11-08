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

export const menu: Story = {
  name: 'A row’s actions on a phone',
  about:
    'What ⋯ opens on compact: the row’s name and its verbs, one row each, the destructive one last and red. It opens at half the height at most; the grabber (pull it up, press it, or the arrow keys on it) takes it to full height, a pull down or ✕ or Escape puts it away.',
  args: { mode: 'menu' },
};

export const filters: Story = {
  name: 'The filters of a list',
  about: 'What *Filters (n)* opens on a phone: the pickers over the results, *Clear* and the act that shows the count at its foot.',
  args: { mode: 'filters' },
};

export const layered: Story = {
  name: 'A dialog opened from inside it',
  about: 'The sheet is on the overlay rank, so Escape closes the dialog first, then the sheet.',
  args: { mode: 'layered' },
};
