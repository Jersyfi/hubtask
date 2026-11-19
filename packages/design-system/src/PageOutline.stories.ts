// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import PageOutlineDemo from './_PageOutlineDemo.svelte';

export default {
  title: 'Wave 7 · Work surface/PageOutline',
  component: PageOutlineDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom'],
} satisfies StoryMeta;

export const page: Story = {
  name: 'Beside a page',
  about: 'The headings of level 2 and 3, the one in view marked with the bar; pressing one scrolls to it and moves the focus there (S-3).',
};
