// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import RecurrenceDemo from './_RecurrenceDemo.svelte';

export default {
  title: 'Wave 3 · Time/RecurrenceEditor',
  component: RecurrenceDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'density', 'zoom', 'width'],
} satisfies StoryMeta;

export const weekly: Story = {
  name: 'Every week, on these days',
  about:
    'The rule the presets produce is printed underneath. The days are drawn starting at the reader’s first day of the week, which comes from their account — a Sunday-first account is not shown a Monday-first grid. Changing the frequency drops the parts that belonged to the old one: `BYMONTHDAY` on a weekly rule is a constraint nothing can satisfy, and a rule nothing satisfies fires never.',
};

export const monthly: Story = {
  name: 'Every month, ending after six times',
  about:
    'The end is a date **or** a count, and the editor writes whichever is chosen while removing the other. That is RFC 5545 §3.3.10 rather than a house rule.',
  args: { mode: 'monthly' },
};

export const raw: Story = {
  name: 'The rule as somebody who reads RFC 5545 writes it',
  about:
    '`BYSETPOS=-1` — the last Friday of the month — has no preset and does not need one. Both paths write the same string, and parts the presets do not know are kept rather than dropped when the rule is edited elsewhere.',
  args: { mode: 'raw' },
};

export const conflict: Story = {
  name: 'A rule that ends twice',
  about:
    '`UNTIL` and `COUNT` together are refused with a field error rather than silently repaired: repairing it would drop one of the two things the author wrote, and neither is safe to guess.',
  args: { mode: 'conflict' },
};
