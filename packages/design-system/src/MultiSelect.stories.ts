// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import MultiSelectDemo from './_MultiSelectDemo.svelte';

export default {
  title: 'Wave 1 · Forms/MultiSelect',
  component: MultiSelectDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'density', 'zoom', 'width'],
} satisfies StoryMeta;

export const empty: Story = {
  name: 'Nothing chosen',
  about:
    'Fifteen options, so `auto` draws the field (ADR-0110). Closed it says the caller’s empty text; Enter, Space or ArrowDown opens the list on its first option. The filter, *All* and *None* are there from eight options.',
  args: { mode: 'empty' },
};

export const some: Story = {
  name: 'Some chosen, as chips',
  about:
    'The chips are a summary of what is chosen, not buttons: one stop in the tab order. Opened, focus lands on the first chosen option; Space toggles and the list stays open; a letter moves to the next option starting with it; Ctrl+A ticks all; Escape closes and puts focus back on the field; Tab walks *All* and *None* and, leaving them, closes the list and goes on.',
  args: { mode: 'some' },
};

export const all: Story = {
  name: 'All chosen, cut after two lines',
  about:
    'Fifteen chips do not fit: the field shows what fits in two lines and the count of the rest. A reader hears every chosen option, not only the ones drawn.',
  args: { mode: 'all' },
};

export const locked: Story = {
  name: 'One option locked on',
  about:
    '*Done or open* has a reason and is ticked: it is drawn with a lock, stands first among the chips, stays focusable in the list, says its reason under its label, and Space, *None* and Ctrl+A leave it ticked.',
  args: { mode: 'locked' },
};

export const refused: Story = {
  name: 'One option refused, with its reason',
  about:
    '*Board column* has a reason and is not ticked: the arrows reach it, its reason is a line under its label - in the list, so it moves nothing - and it cannot be ticked.',
  args: { mode: 'refused' },
};

export const readOnly: Story = {
  name: 'The whole control locked',
  about:
    'A lock set above: the field still opens, read-only, so that what is chosen can be read. Its reason is a bubble at the field and a line at the top of the list; no option toggles and there is no *All* or *None*.',
  args: { mode: 'readonly' },
};

export const invalid: Story = {
  name: 'Needs at least one',
  about:
    'Saved with nothing chosen (design-system.md §4, case 3): a button takes no `aria-invalid`, so the field’s root is marked `data-invalid` and the button is described by the sentence under it - as an upload’s button is. Choose one and press *Save* again: the sentence goes.',
  args: { mode: 'invalid' },
};

export const grouped: Story = {
  name: 'Grouped, with a line per option',
  about:
    'The four groups of what a level carries. Each group is a named `group` inside the listbox; an option’s line describes it and is not part of its name. Type in the filter: a group with no match is left out, and a filter that matches nothing says so, in a live region, and *All* and *None* go until it matches again. The ✕ that empties the filter is named by `clearLabel` - *Clear the filter* - and never by the filter’s own name.',
  args: { mode: 'grouped' },
};

export const summary: Story = {
  name: 'A sentence instead of chips',
  about:
    'The caller’s `summary` says "All except Amount" when nearly all are chosen and leaves the chips otherwise. Untick two more and the chips return.',
  args: { mode: 'summary' },
};

export const german: Story = {
  name: 'Real German labels',
  about:
    'The fifteen of *Was Aufgabe trägt* as `locales/de.json` words them, at the width of a setting’s column. Look at it at the compact width and with +40 % text.',
  args: { mode: 'german' },
};

export const inline: Story = {
  name: 'A few, as visible ticks',
  about:
    'Four options, so `auto` draws ticks - each its own stop, as in `CheckboxGroup`, at its spacing (`size="md"`). Untick both: the group is marked and says once what is missing.',
  args: { mode: 'inline' },
};

export const dense: Story = {
  name: 'Ticks in a setting row',
  about:
    'Five levels at `size="sm"`: the structure panel’s denser rows. *Work packages* is refused with its reason: a bubble at the box on hover, focus and tap.',
  args: { mode: 'dense' },
};

export const inDialog: Story = {
  name: 'Inside a dialog',
  about:
    'Escape closes one layer at a time: the list first, the dialog with its answers stays. At a phone’s width the list is the bottom sheet over the full-screen dialog, and *Done* puts focus back on the field.',
  args: { mode: 'dialog' },
};

export const inRow: Story = {
  name: 'In a row of buttons',
  about:
    'The ticks in a row, drawn with a refused option’s reason and without it: the reason is a bubble at the box and takes no space, and trying it reaches neither the caller nor the row.',
  args: { mode: 'row' },
};

export const inTableRow: Story = {
  name: 'In a table row',
  about: 'The same in a table row: the row keeps its height whether the option is refused or not.',
  args: { mode: 'table' },
};
