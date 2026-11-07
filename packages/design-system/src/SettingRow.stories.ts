// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import SettingRowDemo from './_SettingRowDemo.svelte';

export default {
  title: 'Wave 6 · Compositions/SettingRow',
  component: SettingRowDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'density', 'zoom'],
} satisfies StoryMeta;

export const widths: Story = {
  name: 'Fields as wide as their content',
  about:
    'Name, source and help at the start; the control at `layout.field.xs`, `.sm`, `.md`, `.lg` at the end, *Reset* where set here, the lock’s reason where locked, a bar at the start where the value changed. Stacked on a phone.',
};
