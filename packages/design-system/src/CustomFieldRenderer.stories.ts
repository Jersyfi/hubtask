// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import CustomFieldDemo from './_CustomFieldDemo.svelte';

export default {
  title: 'Wave 3 · Domain/CustomFieldRenderer',
  component: CustomFieldDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'density', 'zoom', 'width'],
} satisfies StoryMeta;

export const kinds: Story = {
  name: 'The eight kinds, and one from a newer server',
  about:
    'The schema has carried these eight since the first migration. `MULTI_SELECT` is a group of checkboxes rather than a multiple `<select>`: the native one is operable in theory and unusable in practice, because holding a modifier while clicking is a gesture most people have never been taught and no touch device offers.',
};

export const user: Story = {
  name: '`USER` is a slot, not an import',
  about:
    'Who may be named in this container is the domain’s answer (F3-07), so the picker is handed in. That is also what keeps this component from depending on the people half of the model to draw the seven kinds that have nothing to do with people.',
  args: { mode: 'user' },
};

export const unknown: Story = {
  name: 'A kind this client has never heard of',
  about:
    'Read-only, with the reason, and **with its value still shown**. Tolerance towards unknown fields is a binding client requirement: a client one window behind the server meets this, which is the normal state of the track rather than an error. Dropping the field would make a value nobody can see indistinguishable from a value that is not there.',
  args: { mode: 'unknown' },
};
