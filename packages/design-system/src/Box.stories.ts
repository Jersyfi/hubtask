// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import BoxDemo from './_BoxDemo.svelte';

export default {
  title: 'Wave 0 · Primitives/Box',
  component: BoxDemo,
  // `draft`: the props are what wave 1 needs today. A primitive turns `stable` when a wave has
  // been built on it and has not asked it for anything it does not have.
  status: 'draft',
  // `dir` because `padding-inline` is the whole reason the prop is not called `paddingX`, and
  // `width` because a padded box is where a narrow viewport first runs out of room. Neither
  // theme nor motion has anything to say about a primitive with no colour and no transition -
  // naming them would be padding the list rather than the box.
  axes: ['dir', 'width', 'zoom'],
} satisfies StoryMeta;

export const everyStep: Story = {
  name: 'Every step of the scale',
  about:
    'The twelve steps, each rendered as the padding around a filled child. Switch Direction to Both: nothing may move, because padding on all four sides has no direction.',
};

export const inlineAndBlock: Story = {
  name: 'Inline padding, which is not left and right',
  about:
    'Padding only along the inline axis. In RTL the wide side changes ends by itself, which is what `padding-inline` buys over `padding-left` — switch Direction to Both and watch the two panes mirror.',
  args: { mode: 'inline' },
};

export const none: Story = {
  name: 'No padding at all',
  about:
    'A Box with no props renders a box-sizing reset and nothing else. This is the story that fails if the primitive ever starts decorating: any colour, border or shadow visible here is a bug.',
  args: { mode: 'bare' },
};
