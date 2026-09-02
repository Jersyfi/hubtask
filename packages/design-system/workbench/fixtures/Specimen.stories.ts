// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The shape every component's stories take from wave 1 onwards. It is Component Story Format
// (ADR-0037): a default export carrying `title` and `component`, named exports carrying `args`.
// `status` and `axes` are the two fields that are ours.

import type { Story, StoryMeta } from '../lib/story.ts';
import Specimen from './Specimen.svelte';

export default {
  title: 'Workbench/Specimen',
  component: Specimen,
  // `fixture`, not `draft`: this is a tool for checking the tool. Nothing consumes it, and wave 1
  // does not build on it.
  status: 'fixture',
  axes: ['theme', 'dir', 'text', 'motion', 'zoom', 'width'],
} satisfies StoryMeta;

export const resting: Story = {
  name: 'Resting',
  about:
    'The whole axis set has something to say about this one. Start with Theme: Both and Direction: Both — four panes, and any rule that holds in one of them and not the others is visible at once.',
};

export const longCopy: Story = {
  name: 'Copy that already wraps',
  about:
    'English that is long before the pseudo-locale touches it. Switch Text to +40 % on top and rule 4 stops being a prediction.',
  args: {
    heading: 'Aufgabenzuordnungsbenachrichtigung',
    body:
      'German is the reason rule 4 has a number in it. This paragraph is already at the width a card gives it; the pseudo-locale then adds the 40 % the rule names, and whatever breaks was going to break in production.',
    unavailableReason:
      'Diese Funktion ist in diesem Arbeitsbereich nicht verfügbar, weil die Fähigkeit für diesen Elementtyp abgeschaltet ist.',
  },
};

export const shortest: Story = {
  name: 'As short as it gets',
  about: 'The other end of rule 4. A layout tuned to long strings collapses here instead.',
  args: { heading: 'Task', body: 'One line.', unavailableReason: 'Off.' },
};
