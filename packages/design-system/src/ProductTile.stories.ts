// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import ProductTileDemo from './_ProductTileDemo.svelte';

export default {
  title: 'Wave 5 · Shell/ProductTile',
  component: ProductTileDemo,
  status: 'draft',
  axes: ['theme', 'zoom'],
} satisfies StoryMeta;

export const products: Story = {
  name: 'The eight products',
  about:
    'Each product’s mark on its label tile, the label’s ink on its surface in both modes; a colour that is not one of the ten draws the plain mark.',
};
