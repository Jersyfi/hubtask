// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import PlaceTabsDemo from './_PlaceTabsDemo.svelte';

export default {
  title: 'Wave 5 · Shell/PlaceTabs',
  component: PlaceTabsDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'motion', 'zoom', 'width'],
} satisfies StoryMeta;

export const hub: Story = {
  name: 'A hub’s tabs',
  about:
    'One strip under a place’s head (user-experience.md §2.4): a navigation of links with `aria-current`, because each tab is a route — the reason `BottomBar` is not `Tabs`. One tab stop; the arrows, Home and End walk the links. Switch the width axis to Compact: the strip scrolls inside itself and keeps the current tab in view, never wrapping.',
};

export const home: Story = {
  name: 'A product home’s views, with a count',
  about: 'A product home’s tabs (§5.1), *Settings* last, a count where something waits.',
  args: { mode: 'home' },
};

export const long: Story = {
  name: 'The same tabs in German',
  about: 'Longer words widen the strip; it scrolls rather than wrapping into a second row.',
  args: { mode: 'long' },
};
