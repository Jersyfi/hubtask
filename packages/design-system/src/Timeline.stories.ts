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
    'A span where both dates exist and a point where only the due date does — the two are different statements, and drawing a bar for a date with no beginning would invent one. The axis is ruled by dated gridlines and today is marked across every row, so a bar is read against a date rather than counted along from an edge. The undated are trayed beside the axis rather than dropped: a timeline that hid what it could not draw would be a filter nobody chose.',
};

export const carried: Story = {
  name: 'A bar is dragged, and an end is dragged alone',
  about:
    'A bar dragged moves both dates and keeps its length; an end dragged moves one. The span redraws where it would land rather than being carried, because what a reader wants to see is the span they would get. A drag names columns and nothing else — what they mean for a date is the application’s (ADR-0063 decision 12). Drag a tray entry across the axis and it is given its first dates. The single-pointer alternative SC 2.5.7 asks for is the row itself, which opens the entry and its date editor.',
};

export const monthScale: Story = {
  name: 'Half a year, ruled by months',
  about:
    'The scale is how far you are looking and how finely it is ruled, together: a fortnight ruled by months says nothing and half a year ruled by days is unreadable. Each column is a day at a width the scale chooses, so the same picture holds a fortnight or six months.',
  args: { scale: 'month' },
};

export const undatedOnly: Story = {
  name: 'Nothing is dated yet',
  about:
    'The axis is empty and the work is not. This is the state a plan starts in, and the tray is the whole of it — which is why it folds. From here an entry is dragged across the axis to be given its first dates, or opened, where `DueDateControl` is what sets them.',
  args: { mode: 'undatedOnly' },
};

export const empty: Story = {
  name: 'A window with nothing in it',
  about:
    'voice-and-tone.md §4.1: name the next step rather than report emptiness.',
  args: { mode: 'empty' },
};
