// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The tokens' visual reference, and the fulfilment of the condition ADR-0037 attached to retiring
// `reference/foundations.html`: the foundations pages are generated from `tokens.json` rather than
// written by hand, so the reference cannot drift from the source it documents.
//
// `fixture`, like the specimen: nothing consumes it and `design-system.md` §4 plans no
// `Foundations` component. It is the tool showing what the tool is built out of.

import type { Story, StoryMeta } from '../lib/story.ts';
import Foundations from './Foundations.svelte';

export default {
  title: 'Foundations/Tokens',
  component: Foundations,
  status: 'fixture',
  axes: ['theme', 'dir', 'text', 'motion', 'zoom', 'width'],
} satisfies StoryMeta;

export const colour: Story = {
  name: 'Colour',
  about:
    'Set Theme to Both. The families are absolute and look the same in either pane; everything semantic changes, which is the point of the layer. The reading pairs are what test/contrast.test.js measures — this page shows them, the test decides whether they are legal.',
  args: { section: 'colour' },
};

export const scale: Story = {
  name: 'Space, radius, type and layers',
  about:
    'Every gap in the product is one of these steps and nothing between. Pull Text to +40 % and the type scale grows with it rather than against it (design-system.md §3 meets SC 1.4.4 through page zoom).',
  args: { section: 'scale' },
};

export const depth: Story = {
  name: 'Depth',
  about:
    'Rule 1: raised is a standalone element, nested is a child, glass is a temporary overlay — no shadow without one of those three reasons. Rule 2 is visible here too: one glass surface, never two.',
  args: { section: 'depth' },
};

export const motion: Story = {
  name: 'Durations and curves',
  about:
    'Switch Motion to Reduced: every bar stops. That is rule 6’s floor, and the axis exists because a preference only the operating system can set is not a user preference (ADR-0037).',
  args: { section: 'motion' },
};

export const mark: Story = {
  name: 'The wordmark, unfinished',
  about:
    'Three nested planes, the innermost in bordeaux. It is not a finished mark — §9 lists it as missing — and it lives here because this is where the page that used to hold it went.',
  args: { section: 'mark' },
};
