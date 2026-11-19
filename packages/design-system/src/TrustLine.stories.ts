// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import TrustLineDemo from './_TrustLineDemo.svelte';

export default {
  title: 'Wave 7 · Work surface/TrustLine',
  component: TrustLineDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom'],
} satisfies StoryMeta;

export const verified: Story = {
  name: 'Verified',
  about: 'Who answers for the page, the verification as a lozenge, the last change, then the labels (S-2).',
};

export const due: Story = {
  name: 'Review due',
  about: 'The verification ran out: the lozenge says so in the warning tone, with its glyph.',
  args: { mode: 'due' },
};

export const ownerless: Story = {
  name: 'Nobody answers for it',
  about: 'Without an owner and never verified, the line says so instead of leaving a gap.',
  args: { mode: 'ownerless' },
};
