// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import TemplateGalleryDemo from './_TemplateGalleryDemo.svelte';

export default {
  title: 'Wave 3 · Hubtask/TemplateGallery',
  component: TemplateGalleryDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom', 'width'],
} satisfies StoryMeta;

export const family: Story = {
  name: 'One family of templates',
  about:
    'A list named by its heading; the cards fill the width in columns of at least `--layout-gallery-card` and fall to one column on a phone, never scrolling sideways.',
};
