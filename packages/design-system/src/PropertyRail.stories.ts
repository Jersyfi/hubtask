// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import PropertyRailDemo from './_PropertyRailDemo.svelte';

export default {
  title: 'Wave 7 · Work surface/PropertyRail',
  component: PropertyRailDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom', 'width'],
} satisfies StoryMeta;

export const story: Story = {
  name: 'A story’s rail (scene task)',
  about: 'The form’s groups as sections in the form’s order, the time product’s section with its tile, the custom fields nobody filled under *More fields*, code and goals folded with their counts. Each head is a twist (aria-expanded). Below `medium` the rail is *All properties (18)* opening a sheet, with three properties above it.',
};

export const ticket: Story = {
  name: 'A ticket’s rail (scene deskentry)',
  about: 'The deadlines on top by urgency, *Triage* under them, the requester as the desk’s section, an empty *Approvals* as one line with “+”.',
  args: { mode: 'ticket' },
};

export const empty: Story = {
  name: 'Mostly empty',
  about: 'Every empty section is one line with “+” (WB-3); a required property is starred and says so to a screen reader.',
  args: { mode: 'empty' },
};
