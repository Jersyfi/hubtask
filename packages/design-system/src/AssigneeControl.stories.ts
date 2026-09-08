// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import AssigneeDemo from './_AssigneeDemo.svelte';

export default {
  title: 'Wave 3 · Domain/AssigneeControl',
  component: AssigneeDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'density', 'zoom', 'width'],
} satisfies StoryMeta;

export const single: Story = {
  name: 'One person, because the installation carries one',
  about:
    'An entry carries `assigneeId` **or** `members[]`, and this is the first shape. Choosing replaces; choosing the person already chosen clears the entry, because "nobody" is a state the model has and a control that could only ever set somebody would make it unreachable.',
};

export const multiple: Story = {
  name: 'A set, because the installation carries one',
  about:
    'The same control and the same keyboard. The only difference is that choosing toggles rather than replaces — which is why this is one component and not two. The summary above the list is a polite live region, so a change announces a name instead of leaving a reader to go looking for what moved.',
  args: { mode: 'multiple' },
};

export const empty: Story = {
  name: 'Nobody to choose from yet',
  about:
    'The candidates are handed in and this component fetches nothing: who may be assigned in a container is the domain’s answer (F3-07). An empty list is therefore a real state rather than a loading one, and voice-and-tone.md §4.1 says what to do about it — name the next step.',
  args: { mode: 'empty' },
};

export const unavailable: Story = {
  name: 'Switched off, with the reason',
  about:
    'There is no `disabled` boolean in this package: `disabledReason` is what disables, and it reaches the accessibility tree through `aria-describedby`. `ErrCapabilityNotSupported` must never become silent ignoring — the CapabilityGate principle, one level down.',
  args: { mode: 'unavailable' },
};
