// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import CalloutDemo from './_CalloutDemo.svelte';

export default {
  title: 'Wave 4 · Documentation/Callout',
  component: CalloutDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom', 'width'],
} satisfies StoryMeta;

export const tones: Story = {
  name: 'The four tones',
  about:
    'A note in prose, true whenever it is read - which is what separates it from Banner: role="note", no live region, no dismissal. Rule 3 twice over: the mark and the start-side rule carry the tone, the prose stays the reading colour.',
};

export const german: Story = {
  name: 'The first note in German',
  about: 'Rule 4: the title wraps, the prose wraps, and the rule on the start side follows dir.',
  args: { isLong: true },
};
