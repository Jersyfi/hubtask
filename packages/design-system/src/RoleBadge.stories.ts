// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import RoleBadgeDemo from './_RoleBadgeDemo.svelte';

export default {
  title: 'Wave 3 · Domain/RoleBadge',
  component: RoleBadgeDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'density', 'zoom', 'width'],
} satisfies StoryMeta;

export const granted: Story = {
  name: 'The role, and where it was granted',
  about:
    'A role applies downwards from its scope (domain-model.md §3.2), so “Admin” says nothing until it says where. This is the pairing the component exists to keep together.',
  args: { label: 'Admin', scopeLabel: 'this workspace' },
};

export const narrower: Story = {
  name: 'The same word, a different reach',
  about:
    'The misreading this prevents: “Admin” beside a name in a collection, understood as the workspace. Same first word, and the second one is the whole difference.',
  args: { label: 'Admin', scopeLabel: 'in Marketing' },
};

export const shared: Story = {
  name: 'One entry, shared',
  about:
    'Sharing an entry with a guest is a membership granted at ITEM scope — it needs no separate mechanism, which is why the badge needs no separate shape for it.',
  args: { label: 'Guest', scopeLabel: 'on this entry' },
};

export const bare: Story = {
  name: 'Where the scope is not in question',
  about:
    'A list that is already one collection’s members says so in its heading. Repeating it on every row is noise, so the scope is optional — but it is the caller who decides that, not the component.',
  args: { label: 'Viewer' },
};

export const ladder: Story = {
  name: 'The whole vocabulary',
  about:
    'Seven roles and no colour between them. Badge’s tones mean a state — fine, not fine, failed — and a role is not a state; seven invented colours would be meaningless to a first-time reader and wrong the day an installation adds an eighth. The word carries the meaning, so the word is what is emphasised.',
  args: { mode: 'ladder' },
};

export const small: Story = {
  name: 'Inside a row',
  about: 'The `sm` size, for a badge that sits inside a list row rather than one that owns its line.',
  args: { label: 'Contributor', scopeLabel: 'in Campaign 2026', size: 'sm' },
};
