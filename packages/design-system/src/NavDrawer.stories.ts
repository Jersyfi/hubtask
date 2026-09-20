// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import NavDrawerDemo from './_NavDrawerDemo.svelte';

export default {
  title: 'Wave 5 · Shell/NavDrawer',
  component: NavDrawerDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'motion', 'density', 'zoom', 'width'],
} satisfies StoryMeta;

export const tree: Story = {
  name: 'The tree, opened from the bar',
  about:
    'Composition and nothing else: a `Drawer` from the start edge, as wide as the pinned navigation (`layout.sidenav.width`; more over a phone), holding the same `SideNav` the desktop pins beside the page. Choosing a node closes it — the caller does that, because the drawer cannot know a navigation happened — and focus returns to the bar’s button. `Escape` and the backdrop are `Drawer`’s. Switch the direction axis: it comes from the start in both, which in Arabic is the right.',
};

export const long: Story = {
  name: 'The same tree in German',
  about:
    'Rule 4 in a fixed width: the drawer is as wide as the pinned navigation and no wider, so a long name wraps inside its node rather than widening the panel over the page. Switch the width axis to Compact — over a phone it takes most of the screen so a name has room, and never more than one and a half times the pinned width.',
  args: { mode: 'long' },
};
