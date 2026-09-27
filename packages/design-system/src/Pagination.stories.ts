// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import PaginationDemo from './_PaginationDemo.svelte';

export default {
  title: 'Wave 2 · Lists/Pagination',
  component: PaginationDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom', 'width'],
} satisfies StoryMeta;

export const middle: Story = {
  name: 'In the middle of a long list',
  about:
    'Pages of a list somebody already holds in full. `total` is a count, and a caller can only pass a count it has — there is no `pageCount` to hand in, no way to say “fetch the fourth page”, and no request anywhere in the file. That is what keeps it on the right side of the rule that this product’s API has no page numbers: a caller holding a cursor has no total to give and reaches for `LoadMore` instead.',
};

export const ends: Story = {
  name: 'At both ends, and the width that does not move',
  about:
    'Four pagers at once: page 1, 2, 19 and 20 of twenty. Count the controls — seven in every one. The window is clipped at the ends, so it grows back inwards; without that the pager changes width as the reader walks through it and the next press lands on the page that slid under the pointer.',
  args: { mode: 'ends' },
};

export const short: Story = {
  name: 'Short lists, and one page',
  about:
    'Four pages need no ellipsis, and a gap is never drawn in place of a single page — a control hiding one page is as wide as the page it hides and adds a press. One page draws no steps at all: the range sentence has already said everything there is to say.',
  args: { mode: 'short' },
};
