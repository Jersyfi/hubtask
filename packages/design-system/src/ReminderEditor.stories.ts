// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import ReminderDemo from './_ReminderDemo.svelte';

export default {
  title: 'Wave 3 · Time/ReminderEditor',
  component: ReminderDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'density', 'zoom', 'width'],
} satisfies StoryMeta;

export const preset: Story = {
  name: 'A preset, which is client vocabulary',
  about:
    '"An hour before" is a phrase and `REL:-PT1H` is the value. The phrases are handed in, because this package writes none. The third channel is one the installation offers and this client has no name for — kept and shown by its value, because a channel nobody can see is a channel nobody can turn off.',
};

export const relative: Story = {
  name: 'Free entry, before the due date',
  about:
    'The same grammar as the presets, with the arithmetic in front of the reader: an amount and a unit produce an ISO 8601 duration. The produced specification is printed underneath so the two paths can be seen agreeing.',
  args: { mode: 'relative' },
};

export const absolute: Story = {
  name: 'Free entry, at a fixed time',
  about:
    'An `ABS:` specification is entered as a **local** time and the caller attaches the zone. The entry’s zone decides when a reminder fires and the recipient’s decides how it reads (i18n-l10n.md §4) — a component that resolved that would be resolving it in the wrong zone half the time.',
  args: { mode: 'absolute' },
};

export const recipients: Story = {
  name: 'Naming who is reminded',
  about:
    'The picker is a slot, so F3-07’s arrives without this component learning what a member is. With nobody named, the sentence in its place says who is reminded anyway: the assignee and the members, resolved when it fires. "Nobody" is what an empty list looks like, and it is not what happens.',
  args: { mode: 'recipients' },
};
