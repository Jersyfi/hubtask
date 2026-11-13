// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import QueryLineDemo from './_QueryLineDemo.svelte';

export default {
  title: 'Wave 7 · Work surface/QueryLine',
  component: QueryLineDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom', 'width'],
} satisfies StoryMeta;

export const filled: Story = {
  name: 'Chips and a text chip in one line (scene qline)',
  about: 'Q-2: four chips and a group with OR as a text chip, no switch. Press a chip to edit its spelling, × to remove it; Backspace in an empty field takes the last chip back. *+ Condition* offers every field, *+ Group (or)* opens a group joined by OR. On a phone the first chips stand in one strip with “+n”, and *Change filter* opens a sheet with a row per condition. The language and the field catalogue are a stub here - Phase 2 hands in the real parser.',
};

export const typing: Story = {
  name: 'Suggestions while typing',
  about: 'Q-3: focus the field: fields with their kind and an example, then - after `priority:` - the values from the data with their counts. ↑↓ choose, ↵ takes; the count slot answers before Enter (Q-4). The list is in the top layer and floats over the rows below; nothing around the line clips it.',
  args: { mode: 'typing' },
};

export const error: Story = {
  name: 'An error at its chip',
  about: 'Q-4: `prio` is not a field; the chip is marked, and the line under it says why with the spelling that fixes it.',
  args: { mode: 'error' },
};

export const empty: Story = {
  name: 'Empty',
  about: 'Nothing typed yet: the placeholder says what can be typed.',
  args: { mode: 'empty' },
};
