// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import TemplateGalleryDemo from './_TemplateGalleryDemo.svelte';

export default {
  title: 'Wave 3 · Hubtask/TemplateCard',
  component: TemplateGalleryDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom', 'width'],
} satisfies StoryMeta;

export const cards: Story = {
  name: 'Three cards: to take, to update, in use',
  about:
    'user-experience.md §5.2: mark, name, one sentence, the products it switches on as chips, a quiet note of where it stands, and the caller’s act at the foot. Never its key or version as the subtitle.',
};

export const long: Story = {
  name: 'Long German words',
  about: 'The name wraps rather than widening the card; the chips wrap under it.',
  args: { mode: 'long' },
};
