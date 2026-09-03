// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import TabsDemo from './_TabsDemo.svelte';

export default {
  title: 'Wave 2 · Structure/Tabs',
  component: TabsDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'motion', 'density', 'zoom', 'width'],
} satisfies StoryMeta;

export const views: Story = {
  name: 'One subject, three views',
  about:
    'One stop in the tab order for the whole strip, and the arrows move within it — the ARIA practices, and the reason a reader does not press Tab six times to get past a single choice. In RTL the arrow that means “next” is the one pointing left, which `rovingIndex` answers rather than this component.',
};

export const gated: Story = {
  name: 'A view that cannot be opened',
  about:
    'There is no `disabled` boolean anywhere in this system: setting `disabledReason` is what switches a control off, so the reason cannot come apart from the state. The arrows skip it rather than landing on something that does nothing.',
  args: { mode: 'gated' },
};

export const long: Story = {
  name: 'The same strip in German',
  about:
    'Rule 4. The strip scrolls rather than squeezing the labels, because a tab whose text is clipped is a tab nobody can choose deliberately.',
  args: { mode: 'long' },
};
