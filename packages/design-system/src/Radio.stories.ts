// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import RadioDemo from './_RadioDemo.svelte';

export default {
  title: 'Wave 1 · Forms/Radio',
  component: RadioDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom'],
} satisfies StoryMeta;

export const resting: Story = {
  name: 'A group, not a button',
  about:
    'A lone radio is a control nobody can switch off, and only a named group gets arrow-key navigation from the browser. The `name` is generated when the caller gives none, so two groups on one page cannot silently become one.',
};

export const gated: Story = {
  name: 'One option that cannot be chosen',
  about: 'Each option may carry its own reason, and the reason is announced with the option rather than with the group.',
  args: { mode: 'gated' },
};
