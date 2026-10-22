// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import SaveBarDemo from './_SaveBarDemo.svelte';

export default {
  title: 'Wave 2 · Structure/SaveBar',
  component: SaveBarDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'motion', 'zoom', 'width'],
} satisfies StoryMeta;

export const form: Story = {
  name: 'A form with changes',
  about:
    'Change a field: the bar arrives over the end of the scroll area with the count, *Discard* and *Save*, and nothing on the page moves. Focus the last field - it scrolls clear of the bar. Undo the change and the bar goes. Save is never switched off for want of a change (UC-WRK-32 check 3).',
};

export const saving: Story = {
  name: 'While it saves',
  about: 'The save keeps its place and its width while it works.',
  args: { mode: 'saving' },
};
