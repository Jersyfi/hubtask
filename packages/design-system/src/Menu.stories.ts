// SPDX-License-Identifier: Apache-2.0
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

export const outside: Story = {
  name: 'Opened from the row it belongs to',
  about:
    'Right-click “Buy milk”, or focus it and press the context-menu key or Shift+F10: the same menu opens at its button with the first item focused, and Escape gives focus back to the row. A right click with Shift keeps the browser’s own menu. A long press on touch opens it the same way (design-system.md §11.9).',
  args: { mode: 'outside' },
};

export const foot: Story = {
  name: 'At the foot of a phone’s screen',
  about:
    'Set the width to compact or the window to a phone, and open the menu on the last row: neither the room above its button nor the room below holds all fifteen items. The menu opens towards the larger room, shrinks to it and scrolls inside, so every item is reachable without scrolling the page.',
  args: { mode: 'foot' },
};
