// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import ButtonDemo from './_ButtonDemo.svelte';

export default {
  title: 'Wave 1 · Forms/Button',
  component: ButtonDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'motion', 'zoom', 'width'],
} satisfies StoryMeta;

export const tones: Story = {
  name: 'The four tones',
  about:
    'Purposes, not states. Hover, press and focus are CSS states on all four — walk the tab order and every one shows the same ring at the same offset (rule 5).',
};

export const german: Story = {
  name: 'The same row in German',
  about:
    'Set Text to +40 % on top of this and pull Width down to compact. Rule 4: the row wraps rather than overflowing, and no label is cut.',
  args: { isLong: true },
};

export const busy: Story = {
  name: 'While it is working',
  about:
    'voice-and-tone.md §2.4: the button keeps its place and its width, and the spinner replaces the icon rather than being added beside it. Switch Motion to Reduced — the turn stops, and `aria-busy` still says what is happening.',
  args: { mode: 'busy' },
};

export const unavailable: Story = {
  name: 'Switched off, with the reason',
  about:
    'There is no `disabled` boolean. Setting `disabledReason` is what disables a button, so a control the reader cannot use cannot come apart from the reason it cannot be used — the CapabilityGate principle, one level down.',
  args: { mode: 'unavailable' },
};

export const inRow: Story = {
  name: 'In a row of buttons',
  about:
    'The reason is a bubble on hover, focus and tap, and takes no space: the row with the reason and the row without it are the same height, and no button beside *Edit* moves. Trying it reaches neither its own handler nor the row around it.',
  args: { mode: 'row' },
};

export const inTableRow: Story = {
  name: 'In a table row',
  about: 'The same in a table row: the row keeps its height whether *Edit* is available or not.',
  args: { mode: 'table' },
};

export const busyInPlace: Story = {
  name: 'Busy, in place',
  about:
    'Switch busy and back: neither button changes its size. Without an icon the spinner is laid over the label, which keeps its place; with one it takes the icon’s.',
  args: { mode: 'busy-in-place' },
};
