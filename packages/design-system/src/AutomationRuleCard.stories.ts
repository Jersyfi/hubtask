// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import AutomationRuleCardDemo from './_AutomationRuleCardDemo.svelte';

export default {
  title: 'Wave 3 · Domain/AutomationRuleCard',
  component: AutomationRuleCardDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'density', 'zoom', 'width'],
} satisfies StoryMeta;

export const rule: Story = {
  name: 'A rule, at a glance',
  about:
    'What somebody scanning a list is scanning for: which rule this is, what starts it, how much it does, whether it is on, and whose rights it acts with. No condition expression and no action parameters — a card that showed them would be unreadable beside the nine rules either side of it.',
};

export const failing: Story = {
  name: 'Before it switches itself off',
  about:
    'Consecutive failures disable a rule by themselves. A rule that went quiet overnight with no warning anywhere is the support thread this line exists to prevent — and the sentence is the caller’s, because “two more” is arithmetic with a plural in it, and plurals belong to the client’s renderer rather than to the catalogue.',
  args: { mode: 'failing' },
};

export const list: Story = {
  name: 'A stack of them',
  about:
    'The labels line up down the column, which is what makes a list scannable rather than read one card at a time. Off is neutral and carries no mark: nothing is wrong with a rule somebody switched off, and a warning colour would say there is.',
  args: { mode: 'list' },
};
