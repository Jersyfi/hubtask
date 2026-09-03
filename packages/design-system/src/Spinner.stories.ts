// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import SpinnerDemo from './_SpinnerDemo.svelte';

export default {
  title: 'Wave 1 · Feedback/Spinner',
  component: SpinnerDemo,
  status: 'draft',
  axes: ['theme', 'motion', 'zoom'],
} satisfies StoryMeta;

export const sizes: Story = {
  name: 'The two sizes',
  about:
    'Switch Motion to Reduced: the turn stops and nothing else changes. Rule 6 is about the movement, not about the information — a spinner that cannot be stopped is what the rule is written for.',
};

export const named: Story = {
  name: 'On its own, with a name',
  about:
    'Standing alone it is the only thing that knows something is coming, so it is a live region and says so. The name is the caller’s resolved text (ADR-0011), in the present participle voice-and-tone.md §2.4 asks for.',
  args: { mode: 'named' },
};

export const inside: Story = {
  name: 'Inside a control that is already busy',
  about:
    'Here the opposite is right: the button carries `aria-busy` and its own name, so the spinner says nothing. Two announcements of one fact is noise, which is why there is no default.',
  args: { mode: 'inside' },
};
