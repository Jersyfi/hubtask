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
    'The ordinary state, and the quiet one: the mark and the word say the copy is in step, the moment says when it last was, and there is no count because there is nothing to count. Rule 3: the green carries nothing the word does not.',
};

export const reconnecting: Story = {
  name: 'Reconnecting, with changes waiting',
  about:
    'The mark turns with the `pending` motion role — a continuous indicator with no end, linear so it never seems to stall — and stands still under reduced motion. The count is a control: opening it lists every waiting change as what and where, in the reader’s own words for the entry.',
  args: { mode: 'reconnecting' },
};

export const offline: Story = {
  name: 'Offline',
  about:
    'The state is a `status` live region (F5-12): losing the server is announced once, when it happens, and the count that moves afterwards is not — a screen reader narrating a heartbeat would be noise. Amber and a word, never amber alone.',
  args: { mode: 'offline' },
};

export const refused: Story = {
  name: 'Changes the server refused',
  about:
    'A rejection is shown, never swallowed (offline-sync.md §9.5): each refused change keeps its what, its where and the reason in voice-and-tone.md’s voice — `sync.gone` says the entry was deleted for good and that what was written is kept; `forbidden` says the person may no longer change it. Dismiss is the only way one leaves the list; a conflict the person may act on carries the resolver’s button beside it.',
  args: { mode: 'refused' },
};

export const rows: Story = {
  name: 'A pending entry in a row and on a card',
  about:
    'What the entry itself says while its change is on its way: the `pending` role on the mark, in opacity alone, and the word beside it — so the row reads without the motion, and the motion stops under the preference. `TaskRow` and `WorkItemCard` take one `pendingLabel` and nothing else changes.',
  args: { mode: 'rows' },
};
