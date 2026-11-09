// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import ResourceGridDemo from './_ResourceGridDemo.svelte';

export default {
  title: 'Wave 6 · Compositions/ResourceGrid',
  component: ResourceGridDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom'],
} satisfies StoryMeta;

export const full: Story = {
  name: 'Templates',
  about:
    'Items flat on the canvas, never inside a section: one column on a phone, two, three, four, five. A card is a link; its act is on the record page.',
};

export const empty: Story = {
  name: 'None yet',
  about:
    'The *unused* empty state with the act that fills the grid.',
  args: { isEmpty: true },
};
