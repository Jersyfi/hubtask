// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import SettingsLayoutDemo from './_SettingsLayoutDemo.svelte';

export default {
  title: 'Pages/SettingsLayout',
  component: SettingsLayoutDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'density', 'zoom'],
} satisfies StoryMeta;

export const full: Story = {
  name: 'Sign-in — full',
  about:
    'The concept’s *SettingsPage*. From `expanded` up the section list beside the open section, sticky while the section scrolls; below `expanded` the list is the page — choose a section and it opens alone with *‹ Sign-in*, the head giving way.',
};

export const section: Story = {
  name: 'Sign-in — a section opened',
  about:
    'The address of a section: on a phone or a tablet the section alone with its way back.',
  args: { start: 'detail' },
};

export const empty: Story = {
  name: 'Sign-in — nothing set here',
  about:
    'Every section at its default.',
  args: { content: 'empty' },
};

export const refused: Story = {
  name: 'Sign-in — refused',
  about:
    'The head stays.',
  args: { content: 'refused' },
};
