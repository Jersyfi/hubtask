// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import IconButtonDemo from './_IconButtonDemo.svelte';

export default {
  title: 'Wave 1 · Forms/IconButton',
  component: IconButtonDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'motion', 'zoom'],
} satisfies StoryMeta;

export const toolbar: Story = {
  name: 'A row of them',
  about:
    'Nothing here is painted text, and every one has a name. Walk the tab order: each announces what it does, because `label` is required and reaches both the accessible name and the tooltip.',
};

export const unavailable: Story = {
  name: 'Switched off, with the reason',
  about: 'As Button: the reason is what disables it, and it is announced through `aria-describedby`.',
  args: { mode: 'unavailable' },
};
