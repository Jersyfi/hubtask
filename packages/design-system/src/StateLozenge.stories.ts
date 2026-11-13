// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import StateLozengeDemo from './_StateLozengeDemo.svelte';

export default {
  title: 'Wave 7 · Work surface/StateLozenge',
  component: StateLozengeDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom'],
} satisfies StoryMeta;

export const vocabularies: Story = {
  name: 'Every vocabulary, one shape',
  about: 'Status categories, SLA, goal, health, agent, rule, release and sync: the same square-cornered lozenge, tone and glyph decided once in `lozenge.ts`. A neutral state carries no glyph; every state whose tone says something also says it with a mark (UX-7).',
};

export const emphasis: Story = {
  name: 'Types, bold and the larger size',
  about: 'An entry type with its own mark; the brand tint for the reader’s own work; `bold` for the one state that must be seen first; `md` where the lozenge stands alone.',
  args: { mode: 'emphasis' },
};

export const long: Story = {
  name: 'Long words',
  about: 'A lozenge never wraps; in a narrow column it ends in an ellipsis and keeps its glyph.',
  args: { mode: 'long' },
};
