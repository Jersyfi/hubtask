// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import ActivityFeedDemo from './_ActivityFeedDemo.svelte';

export default {
  title: 'Wave 3 · Hubtask/ActivityFeed',
  component: ActivityFeedDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'motion', 'density', 'zoom', 'width'],
} satisfies StoryMeta;

export const history: Story = {
  name: 'What happened, newest first',
  about:
    '`verb` is a code and this component never sees one — every sentence arrives resolved, because a feed that wrote “Completed” would be the message catalogue growing a second copy inside a component. The fourth step is a verb this client has never heard of, rendered readably rather than as a key: a client one window behind the server meets one, and that is the normal state of the track rather than an error.',
};

export const compact: Story = {
  name: 'An activity’s compact history',
  about:
    'The verb, the actor and the time are the whole of the step, per the capability matrix. A shorter sentence rather than an empty detail panel — drawing one would invent a gap the model does not have.',
  args: { mode: 'compact' },
};

export const none: Story = {
  name: 'An entry nothing has happened to',
  about:
    'A sentence rather than an empty list. The history is append-only and starts at the entry’s creation, so this is what a reader sees only while the first step is still on its way.',
  args: { mode: 'none' },
};
