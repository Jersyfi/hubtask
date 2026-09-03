// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import AvatarDemo from './_AvatarDemo.svelte';

export default {
  title: 'Wave 1 · Identity/Avatar',
  component: AvatarDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'zoom'],
} satisfies StoryMeta;

export const people: Story = {
  name: 'Names in four scripts',
  about:
    'Where an avatar quietly becomes a Latin-only component. The initials are the first character of the first word and of the last, taken by code point — the least-wrong rule that works in every script. The name is always underneath as the accessible name, whatever the picture shows.',
};
