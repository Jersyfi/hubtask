// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import BannerDemo from './_BannerDemo.svelte';

export default {
  title: 'Wave 1 · Feedback/Banner',
  component: BannerDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom', 'width'],
} satisfies StoryMeta;

export const tones: Story = {
  name: 'The four tones',
  about:
    'In the flow, not over it: a banner moves the content below it and stays until the thing it announces stops being true. Rule 3 — the mark and the words carry the tone, and the text stays the reading colour rather than becoming a paragraph of red.',
};

export const frame: Story = {
  name: 'The two the frame needs',
  about:
    'ADR-0035 §2’s maturity banner and design-system.md §4’s HealthBanner are both a Banner with content, which is why neither is a component of its own. Dismissal is the caller’s: the first is gone for the session, the second comes back while the degradation lasts.',
  args: { mode: 'frame' },
};
