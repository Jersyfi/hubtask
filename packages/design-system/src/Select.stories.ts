// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import SelectDemo from './_SelectDemo.svelte';

export default {
  title: 'Wave 1 · Forms/Select',
  component: SelectDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom', 'width'],
} satisfies StoryMeta;

export const resting: Story = {
  name: 'One of a known few',
  about:
    'A native select, deliberately: the keyboard, the screen reader and the phone picker are the platform’s and none of them is free in a reimplementation. The chevron is ours and is `aria-hidden`, so nothing is announced twice.',
};

export const withUnavailableOption: Story = {
  name: 'An option that cannot be chosen',
  about:
    'The option carries its own reason. The capability principle reaches into the list rather than stopping at the control.',
  args: { mode: 'gated' },
};

export const grouped: Story = {
  name: 'Under headings',
  about:
    'A list long enough that a person scans by what the entries are about before reading them takes `groups`: the native `<optgroup>`, which every platform’s picker draws and every screen reader names. Forty event types are readable this way and not as one column.',
  args: { mode: 'grouped' },
};

export const inRow: Story = {
  name: 'In a row of buttons',
  about:
    'A select, unavailable: it offers only the value it holds. The reason is a bubble at the control on hover, focus and tap, and takes no space: the row with it and the row without it have the same shape. Trying it reaches neither its own handler nor the row.',
  args: { mode: 'row' },
};

export const inTableRow: Story = {
  name: 'In a table row',
  about: 'The same in a table row: the row keeps its height whether the control is available or not.',
  args: { mode: 'table' },
};

export const option: Story = {
  name: 'An option with its reason',
  about:
    'An unavailable option says why in the open list, worded by the caller (`describeUnavailable`). The closed control is sized by the labels alone, so the reason does not widen it: the row with it and the row without it have the same shape.',
  args: { mode: 'option' },
};
