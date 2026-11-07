// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import SignInSection from './_SignInSection.svelte';

export default {
  title: 'Wave 6 · Compositions/SettingsSection',
  component: SignInSection,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'density', 'zoom'],
} satisfies StoryMeta;

export const passwords: Story = {
  name: 'Passwords',
  about:
    'The heading, a sentence and the state on the canvas; one region of setting rows below, quiet group labels between them, *Advanced* folded at the end, and the save bar that stays at the foot while values are changed.',
};

export const locked: Story = {
  name: 'A row locked by the installation',
  about:
    'The lock’s reason beside the control.',
  args: { isLocked: true },
};
