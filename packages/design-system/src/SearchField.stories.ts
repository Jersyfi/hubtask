// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import SearchFieldDemo from './_SearchFieldDemo.svelte';

export default {
  title: 'Wave 2 · Lists/SearchField',
  component: SearchFieldDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'density', 'zoom', 'width'],
} satisfies StoryMeta;

export const labelled: Story = {
  name: 'The input, and nothing else',
  about:
    'What it searches is F2-13’s: a search field that also decided when to send a request would be a second place the debounce and the language live. Escape empties it rather than closing anything — it is the one key a search field is expected to answer, and it does not reach the layer register because a field is not a layer.',
};

export const toolbar: Story = {
  name: 'In a toolbar, its label announced only',
  about:
    'The row is already named, so a second heading above one control would be noise on screen and nothing in the accessibility tree. Hidden, never `display: none` — that would take the name away with the text.',
  args: { mode: 'toolbar' },
};

export const long: Story = {
  name: 'The same field in German',
  about:
    'Rule 4 on a label that is a sentence. The term itself is content and never leaves this component: /search is a POST for exactly that reason, and a field that reflected the term into the address bar would undo it (security.md §9, ADR-0018).',
  args: { mode: 'long' },
};
