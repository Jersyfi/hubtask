// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import WorkPageDemo from './_WorkPageDemo.svelte';

export default {
  title: 'Pages/WorkPage',
  component: WorkPageDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'density', 'zoom'],
} satisfies StoryMeta;

export const full: Story = {
  name: 'Plan — full',
  about:
    'The place\'s tabs, then one toolbar whose segmented control chooses the view; one view at a time, never a second strip.',
};

export const empty: Story = {
  name: 'Plan — nothing planned',
  about:
    'The toolbar stays; the view is the empty state.',
  args: { content: 'empty' },
};

export const refused: Story = {
  name: 'Plan — refused',
  about:
    'The head stays.',
  args: { content: 'refused' },
};
