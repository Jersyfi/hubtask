// SPDX-License-Identifier: BUSL-1.1
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
    'The same treatment for a different question, because it is the same experience: a control the reader cannot use owes them the reason. Which of the two it was is in the sentence rather than in the shape — the reader does not care about the distinction, only about what to do next.',
  args: { mode: 'role' },
};

export const pending: Story = {
  name: 'Before the installation has answered',
  about:
    'The third state, and the one a screen gets wrong. Until /meta/capabilities is read nothing about the installation is known, so a control cannot honestly be shown as available — a screen that showed it anyway offers six actions that disappear a moment later. The mark turns rather than sitting still, so waiting and refusing are not the same picture; reduced motion stops it.',
  args: { mode: 'pending' },
};
