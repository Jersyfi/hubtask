// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The work surface's foundation in one place, for comparing with the concept's mockups. `fixture`:
// nothing consumes it, and design-system.md §4 plans no `WorkSurface` component.

import type { Story, StoryMeta } from '../lib/story.ts';
import WorkSurface from './WorkSurface.svelte';

export default {
  title: 'Wave 7 · Work surface/Gallery',
  component: WorkSurface,
  status: 'fixture',
  axes: ['theme', 'dir', 'text', 'zoom', 'width'],
} satisfies StoryMeta;

export const all: Story = {
  name: 'Every block side by side',
  about: 'The heads of *grammar*, the entry of *task* (head, jump marks, folding sections, composer, rail), the status menus of *wf*, the composer and rail of *deskentry*, the queue, the query line of *qline*, Today’s cards, the service desk’s band and every lozenge. Narrow the window to see each at the tablet and phone widths.',
};

export const grammar: Story = { name: 'Part: heads', args: { part: 'grammar' } };
export const entry: Story = { name: 'Part: an entry', args: { part: 'entry' } };
export const status: Story = { name: 'Part: status', args: { part: 'status' } };
export const composer: Story = { name: 'Part: composer and ticket rail', args: { part: 'composer' } };
export const queue: Story = { name: 'Part: queue', args: { part: 'queue' } };
export const query: Story = { name: 'Part: query line', args: { part: 'query' } };
export const today: Story = { name: 'Part: widgets', args: { part: 'today' } };
export const band: Story = { name: 'Part: product band', args: { part: 'band' } };
export const lozenges: Story = { name: 'Part: lozenges', args: { part: 'lozenges' } };
