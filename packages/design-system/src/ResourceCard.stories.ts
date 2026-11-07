// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import ResourceCardDemo from './_ResourceCardDemo.svelte';

export default {
  title: 'Wave 6 · Compositions/ResourceCard',
  component: ResourceCardDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom'],
} satisfies StoryMeta;

export const cards: Story = {
  name: 'Mark, name, sentence, chips, state',
  about:
    'Level 1′: a hairline and no shadow; the hover a step of surface and a firmer border. A link, a dialog opener, and one that opens nothing.',
};
