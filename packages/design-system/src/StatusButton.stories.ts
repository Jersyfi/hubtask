// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import StatusButtonDemo from './_StatusButtonDemo.svelte';

export default {
  title: 'Wave 7 · Work surface/StatusButton',
  component: StatusButtonDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom', 'width'],
} satisfies StoryMeta;

export const workflow: Story = {
  name: 'With a workflow: from “In progress” on',
  about: 'Press the lozenge (or ↓): every allowed step with its target and what it asks, the step the reader may not take with its reason - focusable, never taken -, how Done is reached, and *View workflow* with where it applies. Arrows walk the steps; Tab reaches *View workflow*; Escape closes and gives focus back. Below `medium` the same rows open as a sheet.',
};

export const free: Story = {
  name: 'Without a workflow',
  about: 'The default: to do, in progress, done, every status reachable, the current one ticked; the done status says it completes the entry.',
  args: { mode: 'free' },
};

export const blocked: Story = {
  name: 'Every step refused',
  about: 'Nothing the reader may take: each step stands with its reason (P-05), none is the next.',
  args: { mode: 'blocked' },
};

export const readonly: Story = {
  name: 'Read only',
  about: 'The status as a lozenge without a menu, and why it cannot be changed here.',
  args: { mode: 'readonly' },
};
