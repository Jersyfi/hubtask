// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import PropertyDemo from './_PropertyDemo.svelte';

export default {
  title: 'Wave 7 · Work surface/Property',
  component: PropertyDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom'],
} satisfies StoryMeta;

export const overview: Story = {
  name: 'Name and value',
  about: 'Two columns, the name a step quieter; an empty value said, a required one starred and named, a note under a computed value, a long value wrapping.',
};
