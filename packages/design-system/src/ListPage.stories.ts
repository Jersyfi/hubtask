// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import ListPageDemo from './_ListPageDemo.svelte';

export default {
  title: 'Pages/ListPage',
  component: ListPageDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'density', 'zoom'],
} satisfies StoryMeta;

export const full: Story = {
  name: 'People — full',
  about:
    'Head with the one primary that creates (full width under the title on a phone, never floating), the one strip of sibling lists, the FilterBar on the canvas, the DataTable as one region with its foot. A row opens the person’s record; *Add people* opens a Dialog M (full screen on a phone). From ⋯ *Select people* the BulkBar takes the head’s place.',
};

export const empty: Story = {
  name: 'People — empty',
  about:
    'Nobody yet: the *unused* empty state inside the region with its act.',
  args: { content: 'empty' },
};

export const refused: Story = {
  name: 'People — refused',
  about:
    'The head stays; what the person may not see says why and who decides.',
  args: { content: 'refused' },
};

export const templates: Story = {
  name: 'Templates — a grid',
  about:
    'The catalogue as a ListPage: the grid of items directly on the canvas under its filter bar — one level, like the People table.',
  args: { list: 'templates' },
};

export const templatesEmpty: Story = {
  name: 'Templates — empty',
  about:
    'No template saved yet.',
  args: { list: 'templates', content: 'empty' },
};
