// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import ApiEndpointCardDemo from './_ApiEndpointCardDemo.svelte';

export default {
  title: 'Wave 4 · Documentation/ApiEndpointCard',
  component: ApiEndpointCardDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom', 'width'],
} satisfies StoryMeta;

export const methods: Story = {
  name: 'The four methods, and a deprecated one',
  about:
    'The method is a Badge whose tone follows what the method does - a read is neutral, a creation succeeds, a change warns, a deletion is the dangerous one - with the word always beside the colour (rule 3). Each card is a section with a heading and an id, so the page is an outline and a link lands on the operation. The path stays left to right under dir=rtl.',
};

export const german: Story = {
  name: 'A summary in German',
  about: 'Rule 4: the summary wraps under the heading rather than being cut to one line.',
  args: { isLong: true },
};
