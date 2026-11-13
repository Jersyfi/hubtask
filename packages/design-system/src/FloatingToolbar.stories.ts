// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import FloatingToolbarDemo from './_FloatingToolbarDemo.svelte';

export default {
  title: 'Wave 7 · Work surface/FloatingToolbar',
  component: FloatingToolbarDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom'],
} satisfies StoryMeta;

export const selection: Story = {
  name: 'Above selected words',
  about: 'The writing bar stands above the selection, centred on it, and flips below where the top has no room (S-1).',
  args: { mode: 'selection' },
};
