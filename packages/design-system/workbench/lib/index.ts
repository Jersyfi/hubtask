// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The index: components in their waves, and a filter over them (ADR-0061 decision 5, F9-03).
//
// The index lists components, not stories. Seventy-five components with two to five stories
// each made two hundred and fifty rows in one column, all open, with no way to find one; a row
// per component with its story count is seventy-five, and the stories of the current component
// are the tabs above the stage, where one switches between them. Nothing about the story format
// changes - this groups what `story.ts` already reads, by the segment before the slash.
//
// These are pure functions over the loaded groups so that `test/workbench.test.js` can hold them
// without a browser: which wave a component is in, what a query matches, and what the current
// component of a story is are questions about lists.

import type { LoadedStory, StoryGroup } from './story.ts';

export interface Wave {
  readonly wave: string;
  readonly components: readonly StoryGroup[];
}

/** The waves in the order `loadStories` sorted them, each with its components. */
export function waves(groups: readonly StoryGroup[]): Wave[] {
  const byWave = new Map<string, StoryGroup[]>();
  for (const group of groups) byWave.set(group.group, [...(byWave.get(group.group) ?? []), group]);
  return [...byWave.entries()].map(([wave, components]) => ({ wave, components }));
}

/** The component a story belongs to, or nothing for an id no story has. */
export function componentOf(groups: readonly StoryGroup[], storyId: string | null): StoryGroup | undefined {
  if (!storyId) return undefined;
  return groups.find((group) => group.stories.some((story) => story.id === storyId));
}

const fold = (value: string) => value.normalize('NFKD').toLowerCase();

/**
 * Whether a component answers a query: by its own name, by its wave, or by the name of one of its
 * stories - "three levels" finds `TaskRow` through the story that carries those words. An empty
 * query matches everything, which is what makes clearing the field the same as never typing.
 */
export function matches(group: StoryGroup, query: string): boolean {
  const needle = fold(query.trim());
  if (!needle) return true;
  if (fold(group.title).includes(needle) || fold(group.group).includes(needle)) return true;
  return group.stories.some((story: LoadedStory) => fold(story.name).includes(needle));
}

/** The waves with only the components the query keeps, and no wave left empty. */
export function filtered(groups: readonly StoryGroup[], query: string): Wave[] {
  return waves(groups)
    .map(({ wave, components }) => ({ wave, components: components.filter((c) => matches(c, query)) }))
    .filter(({ components }) => components.length > 0);
}
