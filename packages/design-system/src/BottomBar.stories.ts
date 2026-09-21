// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import BottomBarDemo from './_BottomBarDemo.svelte';

export default {
  title: 'Wave 5 · Shell/BottomBar',
  component: BottomBarDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'motion', 'zoom', 'width'],
} satisfies StoryMeta;

export const four: Story = {
  name: 'Four destinations, and a count on one',
  about:
    'The primary group of the one navigation list on a phone (ADR-0061): a mark and a word each, `aria-current` on the one the reader is on, a count where something waits. It switches routes, not panels — a `<nav>` of links, not `Tabs`. Three to five and never more: a sixth would be narrower than a thumb. The bar is fixed to the bottom of the viewport, which in the workbench is the pane.',
};

export const keyboard: Story = {
  name: 'It gives way to the keyboard',
  about:
    'A bar fixed to the bottom rides the on-screen keyboard up and covers the field it belongs to. Focus the field: the bar fades — opacity, so nothing moves (rule 6) — and comes back when focus leaves. `:has` on the root is what sees the focus, and it is on the browser row (ADR-0044).',
  args: { mode: 'keyboard' },
};

export const long: Story = {
  name: 'The same four in German',
  about:
    'Rule 4 in a fifth of a phone: a word that does not fit loses its end rather than wrapping into a second line the bar has no height for. Switch the width axis to Compact.',
  args: { mode: 'long' },
};
