// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import SectionNavDemo from './_SectionNavDemo.svelte';

export default {
  title: 'Wave 6 · Compositions/SectionNav',
  component: SectionNavDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom'],
} satisfies StoryMeta;

export const settings: Story = {
  name: 'The sections of Sign-in',
  about:
    'Each section a link with its own address and its state — *2 set here* in the brand colour, *Locked* with the lock. From `expanded` up the list beside the section, the current one marked at its start; narrow the window below `expanded` and it becomes the page: one region of rows with a chevron. Arrows, Home and End move between the links.',
};

export const editor: Story = {
  name: 'An editor’s objects, with search',
  about:
    'The object list of an editor (Workflows): a search over it, states as badges (*live*, *draft*).',
  args: { mode: 'editor' },
};
