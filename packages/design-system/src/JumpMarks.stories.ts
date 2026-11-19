// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import JumpMarksDemo from './_JumpMarksDemo.svelte';

export default {
  title: 'Wave 7 · Work surface/JumpMarks',
  component: JumpMarksDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom', 'width'],
} satisfies StoryMeta;

export const overview: Story = {
  name: 'Jump marks with counts',
  about: 'ET-2: the sections of an entry with their counts, sticking under the bar while the page scrolls; the mark in view is `aria-current="location"`. A press scrolls to the section and moves focus there. Below `medium` the marks are one row that scrolls sideways and act as the switch between sections: the caller shows the chosen one.',
};
