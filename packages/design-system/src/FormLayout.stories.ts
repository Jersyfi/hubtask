// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import FormLayoutDemo from './_FormLayoutDemo.svelte';

export default {
  title: 'Wave 6 · Compositions/FormLayout',
  component: FormLayoutDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom'],
} satisfies StoryMeta;

export const dialog: Story = {
  name: 'In a dialog',
  about:
    'One column, labels above, titled sections, one *Advanced* fold, the primary at the foot (Dialog L: nine fields in three groups).',
};

export const page: Story = {
  name: 'On a page',
  about:
    'The page’s full width, each section a region of setting rows — the look of a settings section; the rare section folded.',
  args: { place: 'page' },
};
