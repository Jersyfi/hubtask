// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import TaskRowDemo from './_TaskRowDemo.svelte';

export default {
  title: 'Wave 3 · Domain/TaskRow',
  component: TaskRowDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'density', 'zoom', 'width'],
} satisfies StoryMeta;

export const levels: Story = {
  name: 'The three levels, expanded',
  about:
    'A task holds work packages, a work package holds activities — domain-model.md §3.4, and the indent is the depth. It mirrors in RTL, because it is `padding-inline-start` and not a left. Completion is a **checkbox**, not an icon button: it is a two-state control a screen reader announces as checked, and its label is announced rather than drawn because the title beside it is what the reader sees.',
};

export const collapsed: Story = {
  name: 'The fourth variant, which is not a fourth type',
  about:
    '§4 asks for four variants and the fourth is the collapsed state — whether the row hides anything, not what kind of entry it is. So `type` says which mark and which indent, and `expansion` says whether there is something behind the twist.',
  args: { mode: 'collapsed' },
};

export const unknown: Story = {
  name: 'A type this client has never heard of',
  about:
    'domain-model.md §2’s extension example is that a new type is a profile entry and no code change. So a type with no mark in the icon set still gets a row and a fallback mark — “tolerant behaviour towards unknown fields” is a binding client requirement, and refusing to draw an entry because its type is new is the opposite of tolerant.',
  args: { mode: 'unknown' },
};

export const long: Story = {
  name: 'The same rows in German',
  about:
    'Rule 4 against the component with the least room: the indent takes width from the title at every level. The title wraps rather than pushing the badge and the menu off the end.',
  args: { mode: 'long' },
};
