// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import ParameterTableDemo from './_ParameterTableDemo.svelte';

export default {
  title: 'Wave 4 · Documentation/ParameterTable',
  component: ParameterTableDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'density', 'zoom', 'width'],
} satisfies StoryMeta;

export const body: Story = {
  name: 'A request body',
  about:
    'A real Table underneath, so a screen reader reads down a column. Required is a word, not an asterisk; an enum and a default are small facts under the type; an object\'s fields sit one level in; a deprecated field is marked and kept, because the reader of an old integration is the one who needs to find it.',
};

export const german: Story = {
  name: 'The same table in German',
  about: 'Every word is handed in, including the four headings and the three verdicts; nothing here knows English.',
  args: { isLong: true },
};
