// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import MenuDemo from './_MenuDemo.svelte';

export default {
  title: 'Wave 1 · Overlays/Menu',
  component: MenuDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'motion', 'zoom'],
} satisfies StoryMeta;

export const actions: Story = {
  name: 'Operable without a pointer',
  about:
    'Tab to the button and press Down: the menu opens on the first item. Arrows walk it and wrap, Home and End jump, Escape closes it and gives focus back to the trigger, and Tab closes it and moves on rather than trapping you in a list of actions you had decided against. The unavailable item keeps its reason and is still reachable — a disabled control the arrows skip is one nobody can ask about.',
};

export const long: Story = {
  name: 'Fifteen items, and type-ahead',
  about:
    'Where arrows alone stop being usable. Open it and press “a” twice: focus walks the items that share the letter, starting after the one it is on, which is what makes a repeated letter walk a group rather than sticking on the first match.',
  args: { mode: 'long' },
};
