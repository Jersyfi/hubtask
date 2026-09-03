// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import ListStatesDemo from './_ListStatesDemo.svelte';

export default {
  title: 'Wave 2 · Lists/EmptyState',
  component: ListStatesDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom', 'width'],
} satisfies StoryMeta;

export const unused: Story = {
  name: 'Nothing has been made yet',
  about:
    'voice-and-tone.md §4.1: say what this place is for and offer the one action that fills it. The only one of the three that carries a call to action.',
  args: { mode: 'unused' },
};

export const filtered: Story = {
  name: 'A filter excluded everything',
  about:
    '§4.2, and never the same copy as §4.1 — “offering to create something when eleven things exist and are hidden is an answer to a question nobody asked”. The mark differs as well as the words, so the two read apart in greyscale (rule 3).',
  args: { mode: 'filtered' },
};

export const settled: Story = {
  name: 'The emptiness is the good outcome',
  about:
    '§4.3: state the fact, offer nothing, and do not celebrate it — an empty list is not an achievement, and §7’s celebrations are for work completed. This story passes an action deliberately and the component drops it: the rule is enforced rather than trusted to every call site.',
  args: { mode: 'settled' },
};
