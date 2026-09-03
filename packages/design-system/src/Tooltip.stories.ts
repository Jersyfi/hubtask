// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import TooltipDemo from './_TooltipDemo.svelte';

export default {
  title: 'Wave 1 · Overlays/Tooltip',
  component: TooltipDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'motion', 'zoom'],
} satisfies StoryMeta;

export const row: Story = {
  name: 'The four sides',
  about:
    'Tab to a button rather than hovering it: the tooltip belongs to focus as much as to the pointer, and Escape dismisses it (SC 1.4.13) without touching anything underneath — it is deliberately not in the layer register. Switch Direction to RTL and `inline-start` turns with the text.',
};

export const edges: Story = {
  name: 'Against the edge',
  about:
    'What ADR-0039 bought: the bubble flips to the other side rather than being cut off, during layout where the browser has anchor positioning and through one measured fallback where it has not.',
  args: { mode: 'edges' },
};
