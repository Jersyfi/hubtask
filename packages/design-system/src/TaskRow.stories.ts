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

export const phone: Story = {
  name: 'The three levels at a phone’s width',
  about:
    'Issue 838: in a 327 px pane a deep row used to break its title one letter per line, because the fixed cells around it left the title no track. Switch the width axis to Compact: the title keeps a few words at least, and the badge and the menu move under it when the line has no room for them — a rule of the row, so it holds in a 400 px detail pane as well as on a phone.',
  args: { mode: 'phone' },
};

export const subtree: Story = {
  name: 'Four levels in a 400 px tree',
  about:
    'An entry’s subtree as the detail pane holds it (ADR-0061 decision 4): the tree is `layout.pane.width` wide, so the indent’s step is the smaller one and stops at the third level — deeper rows keep that indent, and the type mark says the level. Depth comes from the tree, never from a type name: the fourth level here is a type this client has never heard of, at `depth 3`, and no code (arc42 Q-03). A container query on the tree, not a media query; switch the width axis to Desktop and the tree is still 400 px.',
  args: { mode: 'subtree' },
};

export const beside: Story = {
  name: 'One row open beside the list',
  about:
    'From `large` up an entry opens in the detail pane beside its list rather than on its own page (ADR-0061 decision 4). The row it came from stays `aria-current` and carries the rail, and a plain press on a title opens beside — the link keeps its address, so a middle click, a modifier or “open in a new tab” still go to the entry’s page, and a reader still hears a link. Press a title: the current row moves and focus stays in the list.',
  args: { mode: 'beside' },
};
