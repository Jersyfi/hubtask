// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import TourDemo from './_TourDemo.svelte';

export default {
  title: 'Wave 3 · Domain/Tour',
  component: TourDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'motion', 'zoom', 'width'],
} satisfies StoryMeta;

export const step: Story = {
  name: 'A step',
  about:
    'A spotlight on one element of the real interface — the cut-out takes the element’s box through CSS anchor positioning (ADR-0039), no measuring — and a coach mark anchored beside it: a caption, one body line in voice-and-tone.md’s register, the count, and next and skip. The element stays interactive. Focus moves to the mark; Tab cycles the mark’s controls and the element; Escape skips.',
};

export const lastStep: Story = {
  name: 'The last step',
  about:
    'Back is offered where a step comes before, and the last step’s verb is what ends the tour rather than "next". In the application the last step leads to the creation dialog and the tour ends when the entry is completed — the first celebration (§7, §8).',
  args: { step: 2 },
};
