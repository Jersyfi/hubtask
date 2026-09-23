// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import AppBarDemo from './_AppBarDemo.svelte';

export default {
  title: 'Wave 5 · Shell/AppBar',
  component: AppBarDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'density', 'zoom', 'width'],
} satisfies StoryMeta;

export const brand: Story = {
  name: 'The brand, a drawer trigger and the account',
  about:
    'What the bar holds from `medium` up: the way into the navigation at the start, the wordmark, the entry to search, and the account menu at the end — the account group of the one navigation list (ADR-0061). The field is the entry to the search page and not the search itself (ADR-0063 decision 4); a caller that fills it takes Search out of its navigation list, so there is still one visible entry to it. No page action: a page’s actions belong to `PageHeader`.',
};

export const title: Story = {
  name: 'The page title, on a phone',
  about:
    'Below `medium` the bar carries the page’s title, because the page head has no room to say it twice. It is a span and not a heading — every screen has one `h1`, and it belongs to the content. Switch the width axis to Compact: the title is the bar’s one line, and its end is what goes.',
  args: { mode: 'title' },
};

export const rail: Story = {
  name: 'The rail toggle, from expanded up',
  about:
    'Above `expanded` the navigation is pinned and the start control folds it to its marks rather than opening anything. Same slot, other kind, and the caller decides which by width — this component knows nothing about the viewport.',
  args: { mode: 'rail' },
};

export const long: Story = {
  name: 'A long title in German',
  about:
    'Rule 4 against one line: the title takes the room the controls leave and loses its end, never the controls. Switch the direction axis — the ellipsis is at the end of the line in both.',
  args: { mode: 'long' },
};
