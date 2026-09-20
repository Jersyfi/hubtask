// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import DrawerDemo from './_DrawerDemo.svelte';

export default {
  title: 'Wave 2 · Structure/Drawer',
  component: DrawerDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'motion', 'density', 'zoom', 'width'],
} satisfies StoryMeta;

export const end: Story = {
  name: 'From the end edge',
  about:
    'A `<dialog>` like Dialog, for the same four reasons — the top layer, a real focus trap, `inert` behind it, and a backdrop nobody can click through. What differs is what it is for: a dialog is a claim that nothing else matters until it is answered, a drawer is a place to work beside what is on screen, so it is always dismissible. Switch the direction axis: `inline-end` is the other side in Arabic, and the slide flips with it.',
};

export const start: Story = {
  name: 'From the start edge',
  about:
    'The edge is logical, never left and right. A drawer that opened from the left in Arabic would open from the far side of the reading direction, which is exactly what the direction axis exists to catch.',
  args: { mode: 'start' },
};

export const bottom: Story = {
  name: 'From the bottom edge',
  about:
    'The third edge (F8-05): a sheet that rises over a canvas that stays where it was, for a screen too narrow to hold a panel beside it. As tall as its content up to most of the screen, the same one glass surface at a time. Switch the width axis to a phone: this is the shape the rule editor’s inspector takes there.',
  args: { mode: 'bottom' },
};

export const layered: Story = {
  name: 'A dialog opened from inside it',
  about:
    'Escape closes one layer at a time, and the order is the register’s rather than the platform’s: the dialog goes first even though the drawer was opened later, because rank decides before recency. That is why `cancel` is refused here — the browser would close the wrong one.',
  args: { mode: 'layered' },
};
