// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import RunStatusBadgeDemo from './_RunStatusBadgeDemo.svelte';

export default {
  title: 'Wave 3 · Domain/RunStatusBadge',
  component: RunStatusBadgeDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'density', 'zoom', 'width'],
} satisfies StoryMeta;

export const all: Story = {
  name: 'All seven, and why they are seven',
  about:
    'SKIPPED is a condition that did not match — nothing went wrong, the rule did not apply. THROTTLED is the rule protecting the workspace from itself. ABORTED_LOOP is the causation depth stopping a rule that triggered itself. Drawing those three as “failed” would make the runs list lie, and send somebody looking for a defect that is the system working.',
};

export const dryRun: Story = {
  name: 'A rehearsal beside the real thing',
  about:
    'The dry run is a presentation of a run, not an eighth status: the status is what happened, the variant is whether anything was written. Folding it into the enum would double the states and produce a matrix nobody can read. The outline is drawn rather than solid, and the word is there too — a border style alone is a colour argument by another name.',
  args: { mode: 'dry' },
};

export const unknown: Story = {
  name: 'A status this build has never seen',
  about:
    'Tolerance towards unknown values is binding: the badge shows the server’s own token, neutral, rather than guessing at a tone or rendering nothing. A client that dropped it would hide a run that happened.',
  args: { mode: 'unknown' },
};
