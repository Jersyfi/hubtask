// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import CapabilityGateDemo from './_CapabilityGateDemo.svelte';

export default {
  title: 'Wave 3 · Domain/CapabilityGate',
  component: CapabilityGateDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'motion', 'density', 'zoom', 'width'],
} satisfies StoryMeta;

export const permitted: Story = {
  name: 'The control, as it is',
  about:
    'Nothing between the caller and the control: a gate that is open renders its children and adds no wrapper, so a permitted screen has no trace of the gate in it.',
};

export const capability: Story = {
  name: 'The type does not carry it',
  about:
    'domain-model.md §2: “setting a field whose capability is not active for the type produces ErrCapabilityNotSupported — **not** silent ignoring.” A client has three answers and only one is honest. Offering it builds a 422. **Hiding** it tells the reader nothing — and is the tempting one, because a hidden control looks tidy. This is the third: the control stays, is `inert`, and says why.',
  args: { mode: 'capability' },
};

export const role: Story = {
  name: 'The role does not permit it',
  about:
    'The mechanism with a role’s sentence. A control the reader’s role forbids is normally absent rather than gated (P-05); where a page needs it for orientation, the gate is how it says why.',
  args: { mode: 'role' },
};

export const pending: Story = {
  name: 'Before the installation has answered',
  about:
    'The third state, and the one a screen gets wrong. Until /meta/capabilities is read nothing about the installation is known, so a control cannot honestly be shown as available — a screen that showed it anyway offers six actions that disappear a moment later. Until then the content keeps its own boxes, dimmed and inert, with no line of its own, so the answer moves nothing.',
  args: { mode: 'pending' },
};

export const inRow: Story = {
  name: 'In a row of buttons',
  about:
    'A control in a line behind a gate (`isInline`): it keeps its box, and a lock over it carries the reason. The reason is a bubble at the lock on hover, focus and tap, and takes no space: the row with it and the row without it have the same shape. Trying it reaches neither its own handler nor the row.',
  args: { mode: 'row' },
};

export const inTableRow: Story = {
  name: 'In a table row',
  about: 'The same in a table row: the row keeps its height whether the control is available or not.',
  args: { mode: 'table' },
};

export const decidingPermitted: Story = {
  name: 'Deciding, then permitted',
  about:
    'Press *Answer*: while the installation has not answered, the content keeps its own boxes, dimmed and inert, with no line of its own - so the answer moves nothing.',
  args: { mode: 'deciding-permitted' },
};

export const decidingRefused: Story = {
  name: 'Deciding, then refused',
  about: 'Press *Answer*: the reason arrives first, in one line above the content, which keeps its size.',
  args: { mode: 'deciding-refused' },
};
