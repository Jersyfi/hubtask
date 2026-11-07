// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import CreatePageDemo from './_CreatePageDemo.svelte';

export default {
  title: 'Pages/CreatePage',
  component: CreatePageDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'density', 'zoom'],
} satisfies StoryMeta;

export const full: Story = {
  name: 'New access token — full',
  about:
    'The page’s full width, sections as regions of setting rows like a settings section; the primary and *Cancel* in the head; the rare scopes folded. On a phone ✕ stands where the trail was.',
};

export const empty: Story = {
  name: 'New access token — nothing filled in',
  about:
    'The primary says why it cannot create yet.',
  args: { content: 'empty' },
};

export const refused: Story = {
  name: 'New access token — refused',
  about:
    'The head stays.',
  args: { content: 'refused' },
};
