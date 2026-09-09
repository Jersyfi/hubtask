// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import PermissionMatrixDemo from './_PermissionMatrixDemo.svelte';

export default {
  title: 'Wave 3 · Domain/PermissionMatrix',
  component: PermissionMatrixDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'density', 'zoom', 'width'],
} satisfies StoryMeta;

export const matrix: Story = {
  name: 'The matrix this installation enforces',
  about:
    'Read from /meta/capabilities’ roles and never compiled in. A table of what the server says, not a control: changing what a role carries is not an operation this product has — a role is granted, and this says what the grant means.',
};

export const qualifiers: Story = {
  name: 'Why the qualifiers are columns',
  about:
    'Contributor is the row this shape exists for: create is unqualified because a created entry is assigned to its creator, and change is “assigned only”. A tick standing for that would be a lie a reader cannot see, so item_access is words beside the permission columns rather than flattened into one.',
};

export const unknown: Story = {
  name: 'An installation this build has never met',
  about:
    'A role, a permission and a qualifier none of which this client knows. Tolerance towards unknown fields is a binding requirement: the row renders, the unknown permission simply has no column here, and the qualifier is shown as the server’s own token — hiding it would tell the reader the cell means nothing when it means something unread.',
  args: { mode: 'unknown' },
};

export const permissionsOnly: Story = {
  name: 'Without the entry columns',
  about:
    'The `item_access` columns are optional, because a screen that is only asking “who may manage people” should not carry four more columns to say it.',
  args: { mode: 'permissions' },
};
