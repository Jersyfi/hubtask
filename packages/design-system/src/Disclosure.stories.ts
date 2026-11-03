// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import DisclosureDemo from './_DisclosureDemo.svelte';

export default {
  title: 'Wave 2 · Structure/Disclosure',
  component: DisclosureDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom', 'width'],
} satisfies StoryMeta;

export const folded: Story = {
  name: 'Folded',
  about:
    'What is needed later, on the same page (user-experience.md UX-4): a button that says whether it is open and which region it opens, a quiet line saying what is inside, the content arriving through the `reveal` role. A closed fold holds nothing.',
};

export const opened: Story = {
  name: 'Opened',
  about: 'The chevron turns, mirrored in a right-to-left page; the content arrives once, and at once under reduced motion.',
  args: { open: true },
};
