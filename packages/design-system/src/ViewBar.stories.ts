// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import ViewBarDemo from './_ViewBarDemo.svelte';

export default {
  title: 'Wave 5 · Shell/ViewBar',
  component: ViewBarDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'motion', 'zoom', 'width'],
} satisfies StoryMeta;

export const collection: Story = {
  name: 'A collection that runs sprints',
  about:
    'One bar of links with aria-current (§3): one tab stop, the arrows, Home and End; the first view is fixed, the others drag into order; ＋ adds a view; Filter stands at the end. Narrow the width: the tools fold to their marks before the views scroll, then behind one ⋯; they never overlap the views.',
  args: { mode: 'collection' },
};

export const tool: Story = {
  name: 'A hub tool’s tabs',
  about:
    'The tool Plan: Schedule · Releases · Sprints, never a second strip; a menu among the views (the inventory’s Manage ▾) is drawn as one of them.',
  args: { mode: 'tool' },
};

export const home: Story = {
  name: 'A product home',
  about:
    'The shared vocabulary (§4): For you first with its count, Settings last.',
  args: { mode: 'home' },
};

export const compact: Story = {
  name: 'Compact: the chip and its sheet',
  about:
    'On compact the bar is a chip under the title; it opens a sheet with the same views in the same order.',
  args: { mode: 'compact' },
};
