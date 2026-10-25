// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import KbdDemo from './_KbdDemo.svelte';

export default {
  title: 'Wave 1 · Feedback/Kbd',
  component: KbdDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom'],
} satisfies StoryMeta;

export const sizes: Story = {
  name: 'A key, in a table, a footer and a button',
  about:
    'One look for a key wherever a shortcut is shown: the table of *Help and shortcuts* at the default size, a list’s footer and a field at `sm`, and the Enter on a filled button in the button’s own ink.',
};
