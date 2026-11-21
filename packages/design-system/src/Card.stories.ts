// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import CardDemo from './_CardDemo.svelte';

export default {
  title: 'Wave 7 · Work surface/Card',
  component: CardDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom', 'width'],
} satisfies StoryMeta;

export const overview: Story = {
  name: 'One surface beside a widget (scene hub)',
  about: 'A widget, a collection’s card pressed whole and a list of rows to the edge stand on the same surface, hairline and corner, without a shadow, in either theme; only the card pressed whole answers the pointer.',
};
