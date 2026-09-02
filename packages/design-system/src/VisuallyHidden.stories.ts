// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import VisuallyHiddenDemo from './_VisuallyHiddenDemo.svelte';

export default {
  title: 'Wave 0 · Primitives/VisuallyHidden',
  component: VisuallyHiddenDemo,
  status: 'draft',
  // The tab walk is the axis that matters: this component is only observable through focus and
  // through a screen reader, and the workbench can show one of the two.
  axes: ['dir', 'zoom', 'width'],
} satisfies StoryMeta;

export const accessibleName: Story = {
  name: 'The name an icon-only control does not paint',
  about:
    'Nothing here looks different from a button with no name — which is the whole difficulty. Walk the tab order and the button announces "Archive this task".',
};

export const skipLink: Story = {
  name: 'Hidden until it takes focus',
  about:
    'The one legitimate reason to unhide one of these. Walk the tab order: the link paints itself when it is reached and disappears again when focus moves on.',
  args: { mode: 'skip' },
};
