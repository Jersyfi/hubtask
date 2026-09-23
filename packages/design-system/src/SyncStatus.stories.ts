// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import SyncStatusDemo from './_SyncStatusDemo.svelte';

export default {
  title: 'Wave 3 · Domain/SyncStatus',
  component: SyncStatusDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'motion', 'zoom', 'width'],
} satisfies StoryMeta;

export const connected: Story = {
  name: 'Connected, nothing waiting',
  about:
    'The ordinary state, and the quietest the frame gets: one mark in the bar, in the subtle text colour, saying nothing until it is pressed. It was a line of every page that read “Connected” (ADR-0063 decision 5) — a row of the screen spent on the case where nothing is wrong. Everything that line printed is behind the mark: the word, when the copy last synchronised, and what waits. Rule 3: the accessible name is the state in words, so the colour carries nothing on its own.',
};

export const reconnecting: Story = {
  name: 'Reconnecting, with changes waiting',
  about:
    'The mark turns with the `pending` motion role — a continuous indicator with no end, linear so it never seems to stall — and stands still under reduced motion. The count rides the mark, and pressing it lists every waiting change as what and where, in the reader’s own words for the entry.',
  args: { mode: 'reconnecting' },
};

export const offline: Story = {
  name: 'Offline',
  about:
    'The state is a `status` live region (F5-12): losing the server is announced once, when it happens, and the count that moves afterwards is not — a screen reader narrating a heartbeat would be noise. The live region is read whether or not anything is open, because the mark it belongs to says nothing aloud. The one state somebody has to see first is the bold form (ADR-0061): the warning accent under inverse text, and the word inside.',
  args: { mode: 'offline' },
};

export const refused: Story = {
  name: 'Changes the server refused',
  about:
    'A rejection is shown, never swallowed (offline-sync.md §9.5). The mark carries a danger dot so that something to read is visible without a count, and each refused change keeps its what, its where and the reason in voice-and-tone.md’s voice — `sync.gone` says the entry was deleted for good and that what was written is kept; `forbidden` says the person may no longer change it. Dismiss is the only way one leaves the list; a conflict the person may act on carries the resolver’s button beside it.',
  args: { mode: 'refused' },
};

export const unread: Story = {
  name: 'The installation could not be read',
  about:
    'The other half of what this mark is for. `/meta/capabilities` is the one read a Hubtask client configures itself from — item types and their capabilities, the roles, the limits — and a client that has not read it knows none of them: the entry screen draws no fields, and nothing is refused, it is simply unknown. So the mark carries the same danger dot a refused change does, and the panel says what is missing, the server’s own sentence for why, the reference to quote, and the ask-again. It is here rather than on a page because this mark is on every screen, and a retry the reader has to navigate to is a retry for somebody who already knows where to look (issue 1020).',
  args: { mode: 'unread' },
};

export const rows: Story = {
  name: 'A pending entry in a row and on a card',
  about:
    'What the entry itself says while its change is on its way: the `pending` role on the mark, in opacity alone, and the word beside it — so the row reads without the motion, and the motion stops under the preference. `TaskRow` and `WorkItemCard` take one `pendingLabel` and nothing else changes.',
  args: { mode: 'rows' },
};

export const sheet: Story = {
  name: 'On a phone, from the bottom',
  about:
    'Below `medium` there is no room beside a bar’s control, so what the mark opens comes from the edge — the same choice the account menu makes, and the same `Drawer`. Width is the caller’s question and never this component’s: it takes `isSheet` and draws what it is told.',
  args: { isSheet: true },
};
