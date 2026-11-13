// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import FloatingToolbarDemo from './_FloatingToolbarDemo.svelte';

export default {
  title: 'Wave 7 · Work surface/InsertList',
  component: FloatingToolbarDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom'],
} satisfies StoryMeta;

export const offer: Story = {
  name: 'What "/" offers',
  about: 'A listbox in groups under the line: the arrows choose and wrap, Enter inserts, a letter jumps to the next choice starting with it, Escape closes; the keys\' line says so (S-1).',
  args: { mode: 'panel' },
};

export const narrowed: Story = {
  name: 'Narrowed by what is typed',
  about: 'The words typed after "/" keep only the choices whose label or detail holds them, marked in bold and underlined; with none left the list says so.',
  args: { mode: 'panel', query: 'emb' },
};

export const none: Story = {
  name: 'Nothing matches',
  about: 'No choice holds the typed words: the list says so and Enter leaves the line to the editor.',
  args: { mode: 'panel', query: 'xyz' },
};
