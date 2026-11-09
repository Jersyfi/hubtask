// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import DangerZoneDemo from './_DangerZoneDemo.svelte';

export default {
  title: 'Wave 6 · Compositions/DangerZone',
  component: DangerZoneDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom'],
} satisfies StoryMeta;

export const group: Story = {
  name: 'Delete or archive a group',
  about:
    'Each destructive act with what it costs; the button opens the Dialog S that confirms it. One act unavailable, with its reason.',
};
