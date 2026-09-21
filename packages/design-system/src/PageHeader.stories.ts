// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import PageHeaderDemo from './_PageHeaderDemo.svelte';

export default {
  title: 'Wave 5 · Shell/PageHeader',
  component: PageHeaderDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'density', 'zoom', 'width'],
} satisfies StoryMeta;

export const collection: Story = {
  name: 'A collection: one primary action, a filter, and a menu of twelve',
  about:
    'The shape is the rule (ADR-0061): one primary action, at most two secondary, and the rest as the menu’s items — the props’ types leave no slot for a third button. The menu has three groups, the separators are the groups, and trash is last and alone. The notice under the title is the caller’s; the second row is the `ViewSwitcher`. Walk the tab order: the primary action comes before the menu.',
};

export const hub: Story = {
  name: 'A hub: create a collection, and import beside it',
  about:
    'The same head with the hub’s actions: the primary is what a hub is for, import is the one secondary, and the menu holds only what a hub can do — no labels, fields, views, templates or policies, which belong to a collection.',
  args: { mode: 'hub' },
};

export const phone: Story = {
  name: 'Folded, on a phone',
  about:
    'Switch the width axis to Compact. Below `medium` the head folds: the breadcrumb becomes the parent alone as the way up, the secondary actions join the menu behind one control, and the primary action is a round button pinned above the bottom bar where the thumb is. With `isTitleInBar` the bar shows the title, so the `h1` here is read and not drawn — a screen holds exactly one, and the reader is told it once.',
  args: { mode: 'phone' },
};

export const long: Story = {
  name: 'The same head in German',
  about:
    'Rule 4 in the row that has the least room for it: the title wraps and balances, the actions keep their width and wrap under it before anything clips, and the menu items are as long as their words. Switch the direction axis: the menu opens at the end of the line in both.',
  args: { mode: 'long' },
};
