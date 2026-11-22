// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import ObjectHeadDemo from './_ObjectHeadDemo.svelte';

export default {
  title: 'Wave 7 · Work surface/ObjectHead',
  component: ObjectHeadDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom', 'width'],
} satisfies StoryMeta;

export const grammar: Story = {
  name: 'One object, one shape (scene grammar)',
  about: 'Ticket, story, configuration item, goal, agent and release: key with product tile and type, title, the status as a button with its steps (or a lozenge where it is not the reader’s to change), the next step as the primary, then person, due date and the one deadline chip (WB-1).',
};

export const story: Story = {
  name: 'A story, filled (scene task)',
  about: 'The breadcrumb and ⋯ above, the meta line after the type, the title edited in place - press the words or the pencil, Enter keeps, Escape gives the old title back -, the facts and the tools at the bar’s end. On a phone the bar carries the way back and the tools, and the steps live in the status’s sheet.',
  args: { mode: 'story' },
};

export const ticket: Story = {
  name: 'A ticket with a breached deadline (scene deskentry)',
  about: 'Two steps the workflow offers, and one chip: what is breached and the next deadline (ET-6).',
  args: { mode: 'ticket' },
};

export const empty: Story = {
  name: 'Nobody has it',
  about: 'An empty person is said, never a blank; without a workflow the primary is ✓ Done, and once done the secondary *Reopen* with its arrow back.',
  args: { mode: 'empty' },
};

export const readonly: Story = {
  name: 'Read only',
  about: 'No pencil, no steps: the status stands with its reason.',
  args: { mode: 'readonly' },
};

export const long: Story = {
  name: 'A long title',
  about: 'The title wraps; the action bar wraps under it rather than squeezing it.',
  args: { mode: 'long' },
};
