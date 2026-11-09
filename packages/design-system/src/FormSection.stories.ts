// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import FormLayoutDemo from './_FormLayoutDemo.svelte';

export default {
  title: 'Wave 6 · Compositions/FormSection',
  component: FormLayoutDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom'],
} satisfies StoryMeta;

export const page: Story = {
  name: 'Sections on a page',
  about:
    'A heading and a count on the canvas over one region of rows; the folded one is the region’s only line until it is opened.',
  args: { place: 'page' },
};

export const dialog: Story = {
  name: 'Sections in a dialog',
  about:
    'A group with its title over the fields.',
};
