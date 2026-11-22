// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import StateMarkDemo from './_StateMarkDemo.svelte';

export default {
  title: 'Wave 7 · Work surface/StateMark',
  component: StateMarkDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom', 'width'],
} satisfies StoryMeta;

export const forms: Story = {
  name: 'Open, done and a step',
  about: 'The one press of the state control in a row and on a card, the same round mark on every level: empty where one press completes, filled and ticked once done, an arrow where the press takes a step that does not complete the entry. A checkbox to a screen reader where it completes or reopens, a button named by its step otherwise. Hover shows what the press will do.',
};

export const inRows: Story = {
  name: 'In a row, beside the chip',
  about: 'Where the checkbox stood, on every level: alone in a list, before the column or status chip on a board. On a phone or with a coarse pointer the hit area reaches the thumb’s target; the drawn mark stays the same.',
  args: { mode: 'rows' },
};

export const unavailable: Story = {
  name: 'Not pressable, with its reason',
  about: 'Read only, offline for a step, or no step leading on: the edge breaks and the reason is said at the mark. Busy while its write is under way.',
  args: { mode: 'unavailable' },
};
