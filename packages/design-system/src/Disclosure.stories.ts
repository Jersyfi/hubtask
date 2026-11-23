// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import DisclosureDemo from './_DisclosureDemo.svelte';

export default {
  title: 'Wave 2 · Structure/Disclosure',
  component: DisclosureDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom', 'width'],
} satisfies StoryMeta;

export const folded: Story = {
  name: 'Folded',
  about:
    'What is needed later, on the same page (user-experience.md UX-4): a button that says whether it is open and which region it opens, a quiet line saying what is inside, the content arriving through the `reveal` role. A closed fold holds nothing.',
};

export const opened: Story = {
  name: 'Opened',
  about: 'The chevron turns, mirrored in a right-to-left page; the content arrives once, and at once under reduced motion.',
  args: { open: true },
};

export const row: Story = {
  name: 'A row of a form on a phone',
  about:
    '`tone="row"`: a field folded to one bordered row the column’s width, the label at its start and the chevron at its end, as tall as a thumb’s target - *Colour and symbol* on the phone’s *Create hub*, whose mark beside the name keeps the choice in view while it is closed.',
  args: { isRow: true },
};

export const rowWithValue: Story = {
  name: 'A row that shows what is chosen',
  about:
    'A row with `value`: what is chosen inside stands at the row’s end, before the chevron, so a folded choice is seen without opening it - a collection’s colour and symbol in *Create collection*.',
  args: { isRow: true, hasValue: true },
};
