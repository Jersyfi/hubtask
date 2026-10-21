// SPDX-License-Identifier: Apache-2.0
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

export const inRow: Story = {
  name: 'In a row of buttons',
  about:
    'A due date that cannot be changed. It states its reason once - one bubble from the mark at its heading, whichever part is hovered, focused or tried - where it used to print it under every part. The reason is a bubble at the control on hover, focus and tap, and takes no space: the row with it and the row without it have the same shape. Trying it reaches neither its own handler nor the row.',
  args: { mode: 'row' },
};

export const inTableRow: Story = {
  name: 'In a table row',
  about: 'The same in a table row: the row keeps its height whether the control is available or not.',
  args: { mode: 'table' },
};
