// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import DetailPageDemo from './_DetailPageDemo.svelte';

export default {
  title: 'Pages/DetailPage',
  component: DetailPageDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'density', 'zoom'],
} satisfies StoryMeta;

export const full: Story = {
  name: 'Entry — full',
  about:
    'The head, the main column and the details beside it from expanded up; on a phone the details come first, under the head.',
};

export const empty: Story = {
  name: 'Entry — nothing set',
  about:
    'No details column where it would hold no row; the main column says what is missing.',
  args: { content: 'empty' },
};

export const refused: Story = {
  name: 'Entry — refused',
  about:
    'The head stays.',
  args: { content: 'refused' },
};
