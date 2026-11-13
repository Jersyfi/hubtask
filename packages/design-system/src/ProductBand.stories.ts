// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import ProductBandDemo from './_ProductBandDemo.svelte';

export default {
  title: 'Wave 7 · Work surface/ProductBand',
  component: ProductBandDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom', 'width'],
} satisfies StoryMeta;

export const desk: Story = {
  name: 'The service desk’s home (scene deskhome)',
  about: 'The product’s tile, its name, its figures in one line - the first bold -, *New request ▾* as the one primary, ⋯; the four key figures under it as `SummaryCard`s (WB-5). On a phone the band is compact: the smaller tile, two figures, and the primary floats above the bottom bar.',
};

export const all: Story = {
  name: 'Every product (scene bands)',
  about: 'The eight products, each in its own colour through `product.<id>`: the colour identifies and means nothing (WB-2).',
  args: { mode: 'all' },
};

export const empty: Story = {
  name: 'Nothing yet',
  about: 'A product with no content says so in its figure line; the empty state below is `EmptyState`’s.',
  args: { mode: 'empty' },
};

export const timer: Story = {
  name: 'A control of its own (scene time)',
  about: 'Time’s band holds the timer in `aside`, before *Log time*: the product’s own control at the actions’ height, the timer in the time colour where the caller draws it.',
  args: { mode: 'timer' },
};
