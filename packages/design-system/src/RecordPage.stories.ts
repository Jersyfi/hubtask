// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import RecordPageDemo from './_RecordPageDemo.svelte';

export default {
  title: 'Pages/RecordPage',
  component: RecordPageDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'density', 'zoom'],
} satisfies StoryMeta;

export const full: Story = {
  name: 'A group — full',
  about:
    'RecordHead (the way back, the kind, the name, the facts, *Edit*, ⋯), the group’s own lists as regions with their heading, count and *Add*, the facts, the danger zone last.',
};

export const empty: Story = {
  name: 'A group — empty',
  about:
    'A new group: each list says it is empty inside its region.',
  args: { content: 'empty' },
};

export const refused: Story = {
  name: 'A group — refused',
  about:
    'The record’s head stays; the content says why it is not shown.',
  args: { content: 'refused' },
};
