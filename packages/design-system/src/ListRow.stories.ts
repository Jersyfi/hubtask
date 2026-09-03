// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import ListRowDemo from './_ListRowDemo.svelte';

export default {
  title: 'Wave 2 · Lists/ListRow',
  component: ListRowDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'density', 'zoom', 'width'],
} satisfies StoryMeta;

export const link: Story = {
  name: 'A row that goes somewhere',
  about:
    'A row with a destination is a **link**, not a div with a click handler: Enter follows it, and a person can open it in a new tab — which readers do constantly with lists, and which a handler on a div takes away from them. The leading control sits outside the activation, so reordering a row is not a way to open it.',
};

export const select: Story = {
  name: 'A row that chooses',
  about:
    'A row that selects rather than navigates is a **button**, with `aria-pressed`. Two shapes rather than one, because they are two different things to a keyboard and to a screen reader.',
  args: { mode: 'select' },
};

export const plain: Story = {
  name: 'A row that does neither',
  about:
    'No destination and no action, so it is neither a link nor a button and stays out of the tab order entirely. A row that looked interactive and was not would be a stop that does nothing.',
  args: { mode: 'plain' },
};

export const long: Story = {
  name: 'The same rows in German',
  about:
    'Rule 4. The title wraps rather than pushing the badge and the menu off the end — the trailing controls are what a reader reaches for, so they are the last thing that may be lost.',
  args: { mode: 'long' },
};
