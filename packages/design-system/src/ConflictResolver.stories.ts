// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import ConflictResolverDemo from './_ConflictResolverDemo.svelte';

export default {
  title: 'Wave 3 · Domain/ConflictResolver',
  component: ConflictResolverDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'motion', 'zoom', 'width'],
} satisfies StoryMeta;

export const bothWays: Story = {
  name: 'Both versions, and two ways out',
  about:
    'The one case offline-sync.md §5 leaves to the person: a conflict on the notes. The server has already decided — its version is in place, and the one this device wrote is a system comment on the entry, linked below. Keep theirs dismisses. Write mine again is an ordinary PATCH of the field from the current version, performed by the caller: never a merge of the two texts, never an automatic retry. Both texts are drawn as text, never as markup.',
};

export const archived: Story = {
  name: 'When writing again is unavailable',
  about:
    'An archived entry is read-only, so the second way out is switched off with the reason rather than hidden: the person still sees both versions and still has the comment, and the button says why it cannot do more. There is no `disabled` boolean in this package.',
  args: { mode: 'archived' },
};
