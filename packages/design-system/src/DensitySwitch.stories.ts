// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import Demo from './_DensitySwitchDemo.svelte';

export default {
  title: 'Wave 7 · Work surface/DensitySwitch',
  component: Demo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom', 'width'],
} satisfies StoryMeta;

export const all: Story = {
  name: 'Comfortable or compact',
  about: 'The toolbar’s segmented control for a calm table, kept on this device under the list’s key; gone on a phone.',
};
