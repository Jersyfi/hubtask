// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import GridDemo from './_GridDemo.svelte';

export default {
  title: 'Wave 6 · Primitives/Grid',
  component: GridDemo,
  status: 'draft',
  axes: ['dir', 'text', 'zoom'],
} satisfies StoryMeta;

export const resource: Story = {
  name: 'One to five columns',
  about:
    'The counts of a `ResourceGrid`: one column on a phone, two on a tablet, three, four and five above. The widths are the window’s, like the five widths of §6 — narrow the window, not the pane, to see the count change.',
};

export const sparse: Story = {
  name: 'A width that names no count',
  about: 'Only compact and large are named: medium and expanded keep the one column of compact, xlarge keeps the four of large.',
  args: { columns: { compact: 1, large: 4 } },
};
