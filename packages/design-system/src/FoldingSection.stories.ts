// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import FoldingSectionDemo from './_FoldingSectionDemo.svelte';

export default {
  title: 'Wave 7 · Work surface/FoldingSection',
  component: FoldingSectionDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom', 'width'],
} satisfies StoryMeta;

export const overview: Story = {
  name: 'Text from twelve lines, links from five, the newest three comments',
  about: 'WB-3: an empty section is one line - heading, count, sentence and its act. ET-3: a description clamped to twelve of its own lines with *Show more*, which measures itself and is absent for a short text; links cut to five with *Show all 7*; comments cut to the newest three with *10 older comments*. A fold that would hide one row hides nothing. Tab into the clamped text: the link there opens the fold rather than being focused out of sight (2.4.11).',
};

export const short: Story = {
  name: 'Short text: nothing to fold',
  about: 'A description that fits shows no *Show more*.',
  args: { mode: 'short' },
};
