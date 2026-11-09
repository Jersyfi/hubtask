// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import LogPageDemo from './_LogPageDemo.svelte';

export default {
  title: 'Pages/LogPage',
  component: LogPageDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'density', 'zoom'],
} satisfies StoryMeta;

export const full: Story = {
  name: 'Audit — full',
  about:
    'No query form: filters are chips, the time range first; rows grouped by day, words instead of keys; *Show 50 more* at the foot. On a phone *Filters (2)* opens a sheet and the count stays.',
};

export const empty: Story = {
  name: 'Audit — nothing found',
  about:
    'The *filtered* empty state.',
  args: { content: 'empty' },
};

export const refused: Story = {
  name: 'Audit — refused',
  about:
    'The head stays.',
  args: { content: 'refused' },
};
