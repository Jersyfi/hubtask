// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import CelebrationDemo from './_CelebrationDemo.svelte';

export default {
  title: 'Wave 3 · Domain/Celebration',
  component: CelebrationDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'motion', 'zoom', 'width'],
} satisfies StoryMeta;

export const tier1: Story = {
  name: 'Tier 1 — every completion',
  about:
    'The micro-animation on the completed row: a ring that settles and is gone within the `celebration` role’s duration. The slot is inert and off the tree; the one sentence beside it is a `status` region a screen reader hears once. Under reduced motion the ring is a tint — rule 6’s colour change — for the same duration.',
  args: { tier: 1 },
};

export const tier2: Story = {
  name: 'Tier 2 — the last one under its parent',
  about:
    'A sweep of the signature colour across the parent row, once, under `--motion-celebration-tier2-duration`: longer than tier 1, over before anyone could wait for it. Deterministic, read off the hierarchy (§7): the last activity closes a work package, the last work package a task.',
  args: { tier: 2 },
};

export const tier3: Story = {
  name: 'Tier 3 — the rare one',
  about:
    'Marks in both brand colours rising no further than `--motion-celebration-travel` inside a slot no taller than `--motion-celebration-area`, under `--motion-celebration-tier3-duration`. At most one a day per person: a collection or a hub with nothing open, the day’s close, or a first-ever completion. Nothing here is random, and nothing here blocks: the next completion is not delayed.',
  args: { tier: 3 },
};
