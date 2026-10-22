// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import ReadOnlyNoticeDemo from './_ReadOnlyNoticeDemo.svelte';

export default {
  title: 'Wave 2 · Structure/ReadOnlyNotice',
  component: ReadOnlyNoticeDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom', 'width'],
} satisfies StoryMeta;

export const archived: Story = {
  name: 'An archived collection',
  about:
    'One notice at the top of the area, with the one action that changes the state; the rows under it say nothing about it (UC-WRK-32 check 4). Not a banner - it is not dismissed - and not a callout: it is this area, now.',
};

export const role: Story = {
  name: 'A role that reads',
  about: 'The same notice for a reader whose role reads only: what to do is to ask, so there is no action.',
  args: { mode: 'role' },
};

export const plan: Story = {
  name: 'Not in the plan',
  about: 'A feature the plan does not carry, where the area still shows what it holds.',
  args: { mode: 'plan' },
};

export const rows: Story = {
  name: 'Rows that still show what is done',
  about:
    'An archived collection whose rows keep their completion box - it is how a row says done to a screen reader. The boxes share the area’s reason (`provideReasonGroup`): each is `aria-disabled` and described by the notice’s sentence (`textId`), draws no bubble of its own, and a press on it changes nothing.',
  args: { mode: 'rows' },
};
