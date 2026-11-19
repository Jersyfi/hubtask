// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import ComposerDemo from './_ComposerDemo.svelte';

export default {
  title: 'Wave 7 · Work surface/Composer',
  component: ComposerDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom', 'width'],
} satisfies StoryMeta;

export const ticket: Story = {
  name: 'Reply or internal note (scene deskentry)',
  about: 'Two tabs, the arrows between them; who sees the reply beside them; attachments, snippets (also “/” at the start of a line) and the macros - each saying what it does - in the bar; the button says *Send reply*. Ctrl+↵ sends. On a phone the composer docks above the bottom bar.',
};

export const note: Story = {
  name: 'An internal note',
  about: 'Yellow with a lock on the tab and the button, which says *Save note*; who sees it changes to the desk’s people (ET-7, UC-SVC-04).',
  args: { mode: 'note' },
};

export const comment: Story = {
  name: 'A comment (scene task)',
  about: 'One kind of post: no tabs, the button *Comment*. The formatting bar (`format`) stands inside the frame above the field (ET-5); the field grows with the text, no grip. On a phone or a coarse pointer the icons are 44 px targets.',
  args: { mode: 'comment' },
};

export const readonly: Story = {
  name: 'Refused',
  about: 'Writing is not possible here; the composer says why instead of a field.',
  args: { mode: 'readonly' },
};
