// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import LoadMoreDemo from './_LoadMoreDemo.svelte';

export default {
  title: 'Wave 2 · Lists/LoadMore',
  component: LoadMoreDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'density', 'zoom', 'width'],
} satisfies StoryMeta;

export const more: Story = {
  name: 'Another page, appended',
  about:
    'Cursor pagination and **no page numbers** — the API has none, so no component may imply them. It is a control a person presses rather than an infinite scroll, and that is accessibility rather than taste: a list that loads on scroll has no end for a keyboard or a screen reader to reach. Press it and the count is announced in a polite live region; a sighted reader sees five new rows, and without this a screen-reader user hears nothing.',
};

export const end: Story = {
  name: 'The last page',
  about:
    'No dead button at the end of the list. What it says instead is the caller’s, because “that is all of them” is not true of every list — a filtered one wants different words.',
  args: { mode: 'end' },
};

export const busy: Story = {
  name: 'While the page is on its way',
  about:
    'voice-and-tone.md §2.4: the control keeps its place and changes its verb rather than disappearing. A button that vanished while it worked would move everything below it at the moment the reader was aiming at something.',
  args: { mode: 'busy' },
};
