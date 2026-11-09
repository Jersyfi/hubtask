// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import OverviewPageDemo from './_OverviewPageDemo.svelte';

export default {
  title: 'Pages/OverviewPage',
  component: OverviewPageDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'density', 'zoom'],
} satisfies StoryMeta;

export const full: Story = {
  name: 'Administration — full',
  about:
    'One raised focal card (*Getting started*), every other tile a flat region; three columns, two on a tablet, one on a phone.',
};

export const empty: Story = {
  name: 'Administration — a fresh workspace',
  about:
    'Nothing done yet.',
  args: { content: 'empty' },
};

export const refused: Story = {
  name: 'Administration — refused',
  about:
    'The head stays.',
  args: { content: 'refused' },
};
