// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import PopoverDemo from './_PopoverDemo.svelte';

export default {
  title: 'Wave 1 · Overlays/Popover',
  component: PopoverDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'motion', 'zoom', 'width'],
} satisfies StoryMeta;

export const filter: Story = {
  name: 'Anchored, and not a dialog',
  about:
    'Nothing behind it is inert and there is no scrim: the page keeps working while it is open, which is the difference from Dialog. Open it with the keyboard, then Escape — focus lands back on the trigger. `isOpen` is bindable, so the button beside it opens the same surface.',
};

export const nested: Story = {
  name: 'One inside another',
  about:
    'Where the layer register earns its keep. With both open, Escape closes the inner one and leaves the outer one standing — one layer at a time, whatever order they were opened in (design-system.md §6).',
  args: { mode: 'nested' },
};

export const sheet: Story = {
  name: 'A bottom sheet on a phone, with its foot',
  about:
    'Switch Width to Compact and open it: a popover that is not anchored becomes a bottom sheet named with its label, and `sheetActions` is drawn as the sheet\'s foot - a list\'s *Done*, which closes it and puts focus back on the trigger. At any other width it is the anchored popover and the foot is not drawn; it closes by Escape or a press outside.',
  args: { mode: 'sheet' },
};
