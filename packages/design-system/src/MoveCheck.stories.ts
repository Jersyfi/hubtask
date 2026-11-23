// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import MoveCheckDemo from './_MoveCheckDemo.svelte';

export default {
  title: 'Wave 5 · Shell/MoveCheck',
  component: MoveCheckDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom'],
} satisfies StoryMeta;

export const obstacle: Story = {
  name: 'An obstacle with its way out',
  about:
    'The pre-check before Move to… (§5.3): passed checks, one obstacle with the step that resolves it, warnings that hold nothing back. Move stays unavailable, with its reason, until the obstacle is gone - press the step.',
  args: { mode: 'obstacle' },
};

export const clear: Story = {
  name: 'Only warnings left',
  about:
    'Every obstacle resolved: Move is available, the warnings remain.',
  args: { mode: 'clear' },
};

export const framed: Story = {
  name: 'Framed: what a step will do',
  about:
    'Standing on its own in a dialog - a switch-off’s consequences: one bordered group with a hairline between the lines, each text a heading over its quieter detail, each mark the tone’s round mark (a line may bring its own glyph, `pause` for what pauses), a link where the way out is a place to go.',
  args: { mode: 'framed' },
};
