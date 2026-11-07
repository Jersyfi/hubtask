// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import SheetDemo from './_SheetDemo.svelte';

export default {
  title: 'Wave 6 · Overlays/Sheet',
  component: SheetDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'motion', 'zoom'],
} satisfies StoryMeta;

export const menu: Story = {
  name: 'A row’s actions on a phone',
  about:
    'What ⋯ opens on compact: the row’s name and its verbs, one row each, the destructive one last and red. It opens at half the height at most; the grabber (pull it up, press it, or the arrow keys on it) takes it to full height, a pull down or ✕ or Escape puts it away.',
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
