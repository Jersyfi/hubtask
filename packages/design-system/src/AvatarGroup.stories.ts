// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import AvatarDemo from './_AvatarDemo.svelte';

export default {
  title: 'Wave 1 · Identity/AvatarGroup',
  component: AvatarDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'zoom'],
} satisfies StoryMeta;

export const group: Story = {
  name: 'Overlapping, first on top',
  about:
    'Switch Direction to RTL: the row starts at the other edge and the overlap turns with it, because the offset is `margin-inline-start` and the order is document order rather than a `z-index` per avatar — which would be a value per instance, and therefore an inline style ADR-0028 refuses.',
  args: { mode: 'group' },
};
