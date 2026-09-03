// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import ListStatesDemo from './_ListStatesDemo.svelte';

export default {
  title: 'Wave 2 · Lists/Skeleton',
  component: ListStatesDemo,
  status: 'draft',
  axes: ['theme', 'motion', 'density', 'zoom', 'width'],
} satisfies StoryMeta;

export const loading: Story = {
  name: 'The shape of what is coming',
  about:
    'A placeholder takes the height of the row it stands in for, from the same density tokens — switch the density axis and both change together. That is the whole requirement: a list that renders nothing and then rows moves everything below it, and a reader who had started reading loses their place. There is no shimmer, because rule 6 confines motion to opacity and transform and an animated gradient is neither; what it has is a slow pulse through the `pending` role, and reduced motion takes even that.',
};
