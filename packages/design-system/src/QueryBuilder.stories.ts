// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import QueryBuilderDemo from './_QueryBuilderDemo.svelte';

export default {
  title: 'Wave 3 · Hubtask/QueryBuilder',
  component: QueryBuilderDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'motion', 'density', 'zoom', 'width'],
} satisfies StoryMeta;

export const fields: Story = {
  name: 'Built from what the installation reports',
  about:
    'Every field, every comparison and every fixed value below was handed to the component. It spells none of them out, because `query_fields` grows with the installation and a component that knew the list would be the hard-coded one the manifest exists to replace. Changing the field resets the comparison — the operators belong to the field, and a row that kept the old one would be a row the server refuses.',
};

export const absence: Story = {
  name: 'A comparison that takes no value',
  about:
    'The third control is the operator’s answer, not the component’s: one that asks about absence has nothing to compare against, and an empty box beside it would be asking the reader for something that will never be sent.',
  args: { mode: 'absence' },
};

export const nothing: Story = {
  name: 'An installation that reports no filterable field',
  about:
    'Not an error and not an empty box: the manifest reports nothing that can be filtered on, so the editor says so. §4.1 of voice-and-tone.md — the emptiness has a cause, and the sentence names it.',
  args: { mode: 'nothing' },
};
