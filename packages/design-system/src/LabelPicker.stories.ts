// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import LabelDemo from './_LabelDemo.svelte';

export default {
  title: 'Wave 3 · Domain/LabelPicker',
  component: LabelDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'density', 'zoom', 'width'],
} satisfies StoryMeta;

export const picker: Story = {
  name: 'The collection’s labels',
  about:
    'A label belongs to a **collection** (I-W3), so this list is that collection’s and no other. The description sits in the list rather than only in a management screen: it is what makes a colour mean something to somebody who did not choose it, and the moment that matters is the moment somebody is choosing between two that look alike. Rule 3 — the tick carries the selection, because every option is coloured and colour cannot be what says which are on the entry.',
  args: { mode: 'picker' },
};

export const empty: Story = {
  name: 'A collection with no labels yet',
  about:
    'voice-and-tone.md §4.1 and §4.2 are different sentences, and this is the one component where a reader meets both: a collection that has no labels, and a filter that excluded the ones it has. Type into the filter on the story beside this one to see the other.',
  args: { mode: 'empty' },
};

export const long: Story = {
  name: 'The same picker in German',
  about:
    'Rule 4. A chip truncates rather than pushing the description off the row, and the description truncates rather than wrapping the option into two lines — the list is scanned, and a scannable list has one row per label.',
  args: { mode: 'long' },
};
