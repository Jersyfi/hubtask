// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import BreadcrumbDemo from './_BreadcrumbDemo.svelte';

export default {
  title: 'Wave 2 · Structure/Breadcrumb',
  component: BreadcrumbDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'density', 'zoom', 'width'],
} satisfies StoryMeta;

export const deep: Story = {
  name: 'All five levels',
  about:
    'design-system.md §4 asks for `Hub / … / Parent / Current` from `medium` down, and the five levels are the ones domain-model.md §3.4 has. Narrow the width axis to see it collapse — and note that the ellipsis is a control rather than a decoration: a breadcrumb that hid a level with no way to reach it has removed navigation, not saved space.',
};

export const shallow: Story = {
  name: 'A trail with no middle',
  about:
    'Four levels is the shortest trail that has a middle to hide, so this one never collapses however narrow the pane. The rule is about what can be reconstructed, not about the width.',
  args: { mode: 'shallow' },
};

export const long: Story = {
  name: 'The same path in German',
  about:
    'Rule 4 on the component whose job is to fit a path into one line. Each label truncates on its own rather than the trail scrolling, so the last level — the one saying what you are looking at — is never the one pushed off the end.',
  args: { mode: 'long' },
};
