// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import DialogDemo from './_DialogDemo.svelte';

export default {
  title: 'Wave 1 · Overlays/Dialog',
  component: DialogDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'motion', 'zoom'],
} satisfies StoryMeta;

export const confirm: Story = {
  name: 'A question that has to be answered',
  about:
    'Open it from the keyboard and Tab through it: focus is inside and stays there, everything behind it is inert, and closing it — by Escape, by the close button, or by an answer — puts focus back on the button that opened it. It is a native `<dialog>` in the top layer, so no stacking context can put anything over it.',
};

export const layers: Story = {
  name: 'With a popover inside it',
  about:
    'F1-06’s acceptance, in two key presses. The first Escape closes the popover and leaves the dialog standing; the second closes the dialog. The platform would have closed the dialog first, which is why `cancel` is refused and the register in layers.ts decides instead.',
  args: { mode: 'layers' },
};
