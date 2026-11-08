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
