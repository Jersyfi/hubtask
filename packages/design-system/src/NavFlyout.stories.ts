// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import NavSidebarDemo from './_NavSidebarDemo.svelte';

export default {
  title: 'Wave 5 · Shell/NavFlyout',
  component: NavSidebarDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'motion', 'zoom'],
} satisfies StoryMeta;

export const switcher: Story = {
  name: 'The hub quick switch',
  about:
    '▾ beside the hub’s name: a search field, starred hubs, recently visited hubs, All hubs. Escape or a press outside closes it and focus returns to ▾.',
  args: { mode: 'hub' },
};

export const recent: Story = {
  name: 'Recent',
  about:
    'The Recent row opens the last places beside the column.',
  args: { mode: 'workspace' },
};
