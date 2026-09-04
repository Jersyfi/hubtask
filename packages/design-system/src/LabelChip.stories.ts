// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import LabelDemo from './_LabelDemo.svelte';

export default {
  title: 'Wave 3 · Domain/LabelChip',
  component: LabelDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'density', 'zoom', 'width'],
} satisfies StoryMeta;

export const ten: Story = {
  name: 'All ten, and there is no eleventh',
  about:
    '§4 states it in five words — “ten colorToken values, nothing else” — and domain-model.md §3.5 gives the reason: the colour is a token, not a hex, so theming is possible. Each token is a **pair**, background and foreground, measured together by F1-02 for contrast in both themes. One hex could not carry that, which is why a picker with a colour wheel would produce a chip that is unreadable in one of them. Switch the theme axis: every chip stays legible.',
};

export const removable: Story = {
  name: 'On an entry, with a way off',
  about:
    'The remove control is bigger than the glyph inside it — WCAG 2.2 SC 2.5.8’s 24 px floor applies inside a chip too, and the negative margin is what buys the space back from the layout rather than the target giving it up.',
  args: { mode: 'removable' },
};
