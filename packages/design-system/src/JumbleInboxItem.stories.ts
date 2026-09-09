// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import JumbleInboxItemDemo from './_JumbleInboxItemDemo.svelte';

export default {
  title: 'Wave 3 · Domain/JumbleInboxItem',
  component: JumbleInboxItemDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'density', 'zoom', 'width'],
} satisfies StoryMeta;

export const undecided: Story = {
  name: 'Waiting to be decided about',
  about:
    'NEW is the one state that wants attention, and the only one with a tone. What to do about it is the caller’s slot: this component performs nothing.',
};

export const hostile: Story = {
  name: 'A subject that is a script tag',
  about:
    'The story that matters. An intake address is public — anybody who learns it can put a string in front of a reader — so the raw subject, body and sender are drawn as characters. There is no `{@html}` in this package and conventions.test.js refuses one, which is why this is a rule rather than a habit. The second card is the other outside-content problem: one unbroken string long enough to widen the card if nothing broke it.',
  args: { mode: 'hostile' },
};

export const processed: Story = {
  name: 'Made into an entry',
  about:
    'The link to what the conversion produced is the other half of the provenance pair: the entry names the arrival, and the arrival names the entry.',
  args: { mode: 'processed' },
};

export const dismissed: Story = {
  name: 'Dismissed, and still here',
  about:
    'A dismissal is a state and not a deletion — the entry stays readable and ages out by retention rule. So the card says that, rather than fading towards invisible: a greyed-out row is a client drawing a deletion the server did not perform.',
  args: { mode: 'dismissed' },
};
