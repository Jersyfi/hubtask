// SPDX-License-Identifier: Apache-2.0
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
    'From `large` up an entry opens next to where it was chosen (ADR-0061): over the list, below the bar, so the list keeps its width; the chosen row stays selected. An `aside` with its own heading; it takes no focus and traps none — choose another row and the panel follows. Its start edge is a `ResizeHandle` the person drags, and the caller keeps the width. *Open whole* leads to the entry’s own page; the switch beside it makes every entry open whole from now on.',
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
    'Rule 4 at `layout.panel.width`, the width it opens at: a long title wraps inside its head and the controls beside it keep their place. Switch the direction axis: the panel sits at the end of the line in both, with its edge and its hairline at the start.',
  args: { mode: 'long' },
};
