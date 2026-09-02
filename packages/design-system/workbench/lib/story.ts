// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The story format, and the discovery that finds every story.
//
// The shape is Component Story Format, deliberately (ADR-0037): a default export carrying
// `title` and `component`, and named exports carrying `args`. Two fields are ours - `status` and
// `axes` - and a migration to Storybook, if the trade this project made ever stops being the
// right one, moves those two into `parameters` and renames nothing else.

import type { Component } from 'svelte';

import { AXIS_IDS, type AxisId } from './axes.ts';

/**
 * Where a component stands. The vocabulary is ADR-0035's, applied one level down: the
 * application says it is `experimental` as a whole, and a reader of the workbench wants to know
 * which of the parts inside that are settled.
 */
export type Status = 'fixture' | 'draft' | 'stable';

/**
 * A story's component takes whatever its own `args` declare, and the workbench cannot know more
 * than that: it renders fifty different prop shapes through one stage. The looseness is
 * confined to this one alias rather than spread across the frame.
 */
export type StoryComponent = Component<Record<string, any>>;

export interface StoryMeta {
  /** `Wave 1 · Forms/Button` — the segment before the slash groups the sidebar. */
  readonly title: string;
  readonly component: StoryComponent;
  readonly status: Status;
  /**
   * The axes this component must be seen in before anyone calls it finished. Not a filter: the
   * bar always offers every axis. It is the author's statement of which ones carry a rule for
   * this component, and check-stories.js refuses a name that is not an axis.
   */
  readonly axes: readonly AxisId[];
}

export interface Story {
  readonly name: string;
  readonly args?: Record<string, unknown>;
  /** One sentence on what this story is for. The workbench shows it above the stage. */
  readonly about?: string;
}

export interface LoadedStory extends Story {
  readonly id: string;
  readonly meta: StoryMeta;
}

export interface StoryGroup {
  readonly group: string;
  readonly title: string;
  readonly meta: StoryMeta;
  readonly stories: readonly LoadedStory[];
}

/** `Wave 1 · Forms/Button` + `withIcon` → `wave-1-forms-button--with-icon`. */
export function storyId(title: string, exportName: string): string {
  const slug = (value: string) =>
    value
      .replace(/([a-z0-9])([A-Z])/g, '$1-$2')
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/^-+|-+$/g, '');
  return `${slug(title)}--${slug(exportName)}`;
}

const isMeta = (value: unknown): value is StoryMeta =>
  typeof value === 'object' &&
  value !== null &&
  typeof (value as StoryMeta).title === 'string' &&
  typeof (value as StoryMeta).component === 'function';

/**
 * Two globs, and the second is not a convenience. Components live in src/, which stays empty
 * until wave 1 builds it (ADR-0029); the specimen that proves the workbench works is a fixture of
 * the workbench, not a component pretending to be one.
 */
export function loadStories(): StoryGroup[] {
  const modules = {
    ...import.meta.glob('../../src/**/*.stories.ts', { eager: true }),
    ...import.meta.glob('../fixtures/**/*.stories.ts', { eager: true }),
  } as Record<string, Record<string, unknown>>;

  const groups: StoryGroup[] = [];
  for (const [path, module] of Object.entries(modules)) {
    const meta = module.default;
    if (!isMeta(meta)) throw new Error(`${path}: the default export is not a story meta`);
    for (const id of meta.axes) {
      if (!(AXIS_IDS as readonly string[]).includes(id)) throw new Error(`${path}: unknown axis ${id}`);
    }

    const stories: LoadedStory[] = [];
    for (const [exportName, value] of Object.entries(module)) {
      if (exportName === 'default' || typeof value !== 'object' || value === null) continue;
      const story = value as Story;
      stories.push({ ...story, id: storyId(meta.title, exportName), meta });
    }
    if (stories.length === 0) throw new Error(`${path}: a story module with no story exports`);

    const [group = meta.title, title = meta.title] = meta.title.split('/');
    groups.push({ group, title, meta, stories });
  }

  return groups.sort((a, b) => a.meta.title.localeCompare(b.meta.title));
}
