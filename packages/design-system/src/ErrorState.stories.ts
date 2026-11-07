// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import ListStatesDemo from './_ListStatesDemo.svelte';

export default {
  title: 'Wave 2 · Lists/ErrorState',
  component: ListStatesDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom', 'width'],
} satisfies StoryMeta;

export const failed: Story = {
  name: 'A list that could not be read',
  about:
    'Its own component because voice-and-tone.md §4.4 says a failure is **not** an empty state: “a failure rendered as ‘no results’ is a lie the reader acts on”. It names the fix (§3.1), offers the retry, and shows the reference — an internal error without its request_id is a support thread that cannot be traced (ADR-0025). It is a live region, because a list quietly becoming a paragraph is a change nobody is told about.',
  args: { mode: 'failed' },
};

export const updated: Story = {
  name: 'A screen from an older release',
  about:
    'After an upgrade a tab opened before it asks for a screen whose code the server no longer has. Nobody did anything wrong, so it reads as news (`kind="updated"`): the new release is there, and *Reload* - the whole remedy - is the primary act (user-experience.md §5.8).',
  args: { mode: 'updated' },
};
