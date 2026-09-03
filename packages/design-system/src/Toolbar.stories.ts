// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import ToolbarDemo from './_ToolbarDemo.svelte';

export default {
  title: 'Wave 2 · Structure/Toolbar',
  component: ToolbarDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'density', 'zoom', 'width'],
} satisfies StoryMeta;

export const mixed: Story = {
  name: 'A button, a filter and a sort',
  about:
    'One stop in the tab order for the row, and the arrows move within it — the same trade the tab strip makes, and for the same reason: eight tabbable icon buttons put eight presses between a keyboard reader and the content under them. The controls are read from the DOM at the moment a key is pressed, not held, because a toolbar’s contents change with what is selected.',
};

export const icons: Story = {
  name: 'Icon-only, including one that is off',
  about:
    'Every icon-only control carries its accessible name, which is what `VisuallyHidden` exists for and what makes this row readable at all without sight. The one that is off carries its reason rather than a boolean.',
  args: { mode: 'icons' },
};

export const long: Story = {
  name: 'The same row in German',
  about:
    'Rule 4. It wraps onto a second line rather than scrolling: a toolbar that scrolled would hide actions behind a gesture, and the arrows would then move focus to something off screen.',
  args: { mode: 'long' },
};
