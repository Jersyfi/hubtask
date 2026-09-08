// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import TimelineDemo from './_TimelineDemo.svelte';

export default {
  title: 'Wave 3 · Time/Timeline',
  component: TimelineDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'density', 'zoom', 'width'],
} satisfies StoryMeta;

export const mixed: Story = {
  name: 'Spans, points, and what cannot be placed',
  about:
    'A span where both dates exist and a point where only the due date does — the two are different statements, and drawing a bar for a date with no beginning would invent one. The undated are listed beside the axis rather than dropped: a timeline that hid what it could not draw would be a filter nobody chose.',
};

export const undatedOnly: Story = {
  name: 'Nothing is dated yet',
  about:
    'The axis is empty and the work is not. This is the state a plan starts in, and the entries are still reachable — from here, `DueDateControl` is what puts one on the axis. There is no drag: a bar dragged is a date changed, and F2-12 puts the command before the gesture.',
  args: { mode: 'undatedOnly' },
};

export const empty: Story = {
  name: 'A window with nothing in it',
  about:
    'voice-and-tone.md §4.1: name the next step rather than report emptiness.',
  args: { mode: 'empty' },
};
