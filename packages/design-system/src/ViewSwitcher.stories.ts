// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import ViewSwitcherDemo from './_ViewSwitcherDemo.svelte';

export default {
  title: 'Wave 3 · Hubtask/ViewSwitcher',
  component: ViewSwitcherDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'motion', 'density', 'zoom', 'width'],
} satisfies StoryMeta;

export const built: Story = {
  name: 'The layouts this client draws',
  about:
    'A radio group rather than a tab strip, and the difference is what the control means: a tab switches between subjects and owns the panel it reveals, this switches between renderings of one subject and owns nothing. The arrows move and choose in the same press, which is the ARIA practice for a radio group and what a reader expects of a segmented control.',
};

export const reported: Story = {
  name: 'A layout the installation reports and this client cannot draw',
  about:
    'Shown with the reason rather than left out. Leaving it out would make the switcher disagree with the manifest, and a reader who has heard of the timeline would be looking for something simply absent. The arrows skip it — a radio group moves the selection as it moves focus, so stopping on it would mean choosing it.',
  args: { mode: 'reported' },
};

export const long: Story = {
  name: 'The same switcher in German',
  about:
    'Rule 4: everything grows by 40 %. The group wraps rather than clipping a label, because a view whose name is cut off is a view nobody chooses deliberately.',
  args: { mode: 'long' },
};
