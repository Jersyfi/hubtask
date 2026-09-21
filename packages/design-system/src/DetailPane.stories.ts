// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import DetailPaneDemo from './_DetailPaneDemo.svelte';

export default {
  title: 'Wave 5 · Shell/DetailPane',
  component: DetailPaneDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'density', 'zoom', 'width'],
} satisfies StoryMeta;

export const beside: Story = {
  name: 'An entry open beside its list',
  about:
    'From `large` up an entry opens next to where it was chosen (ADR-0061): the list stays, the chosen row stays selected, and the pane holds the same one-column form the entry takes on a phone. An `aside` with its own heading; it takes no focus and traps none — choose another row and the pane follows. `Open as a page` is a link to the entry’s own address, where everything here can also be done.',
};

export const layered: Story = {
  name: 'Escape closes one layer at a time',
  about:
    'The pane is the weakest dismissible layer. Open the menu inside it and press `Escape`: the menu goes first and the pane stays; press it again and the pane closes. The register decides (layers.ts), not the order things were opened in.',
  args: { mode: 'layered' },
};

export const long: Story = {
  name: 'The same pane in German',
  about:
    'Rule 4 in `layout.pane.width`: the pane is as wide as the token says and no wider, so a long title wraps inside its head and the controls beside it keep their place. Switch the direction axis: the pane sits at the end of the line in both, with its hairline at the start.',
  args: { mode: 'long' },
};
