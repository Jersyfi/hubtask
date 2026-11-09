// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import StandalonePageDemo from './_StandalonePageDemo.svelte';

export default {
  title: 'Pages/StandalonePage',
  component: StandalonePageDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'density', 'zoom'],
} satisfies StoryMeta;

export const full: Story = {
  name: 'Sign-in card — full',
  about: 'No frame: one centred card on the canvas with the two ambient washes, the legal links under it.',
};

export const empty: Story = {
  name: 'Sign-in card — one step',
  about: 'The card holds only what the step asks.',
  args: { content: 'empty' },
};

export const refused: Story = {
  name: 'Sign-in card — refused',
  about: 'One refusal in the banner slot, above the heading.',
  args: { content: 'refused' },
};

export const shared: Story = {
  name: 'Public — full',
  about: 'A page opened by a public link: its title and the content at the window\'s full width.',
  args: { variant: 'public' },
};

export const sharedGone: Story = {
  name: 'Public — gone',
  about: 'A dead link says only that there is nothing here.',
  args: { variant: 'public', content: 'refused' },
};
