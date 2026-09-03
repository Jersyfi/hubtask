// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import PopoverDemo from './_PopoverDemo.svelte';

export default {
  title: 'Wave 1 · Overlays/Popover',
  component: PopoverDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'motion', 'zoom'],
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
