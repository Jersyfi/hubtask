// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import DueDateDemo from './_DueDateDemo.svelte';

export default {
  title: 'Wave 3 · Time/DueDateControl',
  component: DueDateDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'density', 'zoom', 'width'],
} satisfies StoryMeta;

export const timed: Story = {
  name: 'A date and a time, in the reader’s own zone',
  about:
    'The zone is stored beside the instant and shown only when it is not the reader’s — here it is theirs, so there is nothing to say. The platform draws the calendar and the clock, which is what makes both work with a screen reader in every locale.',
};

export const allDay: Story = {
  name: 'All day is a date, never a midnight',
  about:
    '`due_date_only` is a day in the entry’s zone (i18n-l10n.md §4). Rendering it as 00:00 would turn "Thursday" into an instant that is Wednesday for half the world, so the time is absent rather than zero and the control does not offer one.',
  args: { mode: 'allDay' },
};

export const foreign: Story = {
  name: 'A time in somebody else’s zone',
  about:
    'A 09:00 meeting in São Paulo read in Berlin is 14:00, and saying so is the entire reason the zone is stored. The component decides **whether** to say it by comparing the two zones; the sentence itself is the caller’s, resolved, because this package writes none.',
  args: { mode: 'foreign' },
};

export const none: Story = {
  name: 'No due date, and a control that removes one',
  about:
    'Clearing is a button rather than an emptied field: an empty date reads as "I have not typed it yet" as easily as "there is no due date", and one of those is a state worth saving.',
  args: { mode: 'none' },
};
