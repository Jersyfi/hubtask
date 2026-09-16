// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import AISuggestionDemo from './_AISuggestionDemo.svelte';

export default {
  title: 'Wave 3 · Domain/AISuggestion',
  component: AISuggestionDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'motion', 'density', 'zoom', 'width'],
} satisfies StoryMeta;

export const kinds: Story = {
  name: 'Every kind of proposal',
  about:
    'One component for a title, a breakdown, labels, a summary, a translation and a template draft: the kind is the heading, the payload is the caller’s slot rendered with the editor the product already has for its shape, and accept and dismiss are the caller’s buttons. The heading says it is a proposal (voice-and-tone.md §7.1) and the model is one collapsed line (§7.2). Seen in both modes and both directions: the ai.* surface differs from the entry’s by hue and the border carries the boundary, because rule 3 says a hue never stands alone.',
};

export const pending: Story = {
  name: 'Still being made',
  about:
    'The job is running. The region is aria-busy, the label is the present participle of §2.4, and there is no payload and no provenance because there is nothing to show and nothing to name yet. A strip that appeared only when the job finished would be a strip that arrives from nowhere.',
  args: { mode: 'pending' },
};

export const stale: Story = {
  name: 'The entry moved since',
  about:
    'suggestion.stale is the server’s refusal; the caller sets the state here so that the strip says so before the server has to. It offers the ask and the dismissal and never the apply (§7.3), and it is a sentence with a mark rather than a colour.',
  args: { mode: 'stale' },
};

export const german: Story = {
  name: 'The labels in German',
  about:
    'Rule 4: the heading, the disclosure’s label and the buttons all grow by 40 % or more, and the strip wraps rather than clips. With the motion axis set to Reduced the strip shows no transition at all - the arrival is the attach role in opacity, and rule 6’s floor is its absence.',
  args: { mode: 'german' },
};
