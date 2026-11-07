// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import EditorPageDemo from './_EditorPageDemo.svelte';

export default {
  title: 'Pages/EditorPage',
  component: EditorPageDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'density', 'zoom'],
} satisfies StoryMeta;

export const full: Story = {
  name: 'Workflows — full',
  about:
    'The object list with its search beside the editor, a wide gutter between them; the draft bar sticky at the editor’s top; statuses and steps one line each — a status opens a Dialog M, a step a Dialog L. Below `expanded` the list is the page and a workflow opens alone with *‹ Workflows*.',
};

export const object: Story = {
  name: 'Workflows — an object opened',
  about:
    'The address of one workflow on a phone or a tablet.',
  args: { start: 'detail' },
};

export const empty: Story = {
  name: 'Workflows — empty',
  about:
    'No workflow yet.',
  args: { content: 'empty', start: 'detail' },
};

export const refused: Story = {
  name: 'Workflows — refused',
  about:
    'The head stays.',
  args: { content: 'refused' },
};
