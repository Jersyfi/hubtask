// SPDX-License-Identifier: Apache-2.0
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

export const inRow: Story = {
  name: 'In a row of buttons',
  about:
    'The reason shows as a bubble on hover, focus and tap, and takes no space: the row with the reason and the row without it are the same height, and nothing beside the arrow moves. Trying it reaches neither its own handler nor the row.',
  args: { mode: 'row' },
};

export const inTableRow: Story = {
  name: 'In a table row',
  about: 'The same in a table row: the row keeps its height whether the arrow is available or not.',
  args: { mode: 'table' },
};

export const busy: Story = {
  name: 'While it is working',
  about: 'The spinner takes the icon’s place, so the button keeps its size.',
  args: { mode: 'busy' },
};
