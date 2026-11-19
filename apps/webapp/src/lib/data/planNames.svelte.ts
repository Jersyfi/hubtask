// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The names of a hub's releases and of its collections' sprints, for a query line (§6.11, Q-2):
 * the line and its chips say `plan.release: Spring`, the read sends the release's id (ADR-0102,
 * "names are the client's"). Read only while `wanted` - a line naming one, a grouping by release -
 * so a screen that never asks reads nothing more. Called while a component initialises.
 */

import { untrack } from 'svelte';
import type { ReleaseList, SprintList } from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { releasesPath } from './releases.svelte.ts';
import { sprintsPath } from './sprints.svelte.ts';

export interface PlanNamesScope {
  readonly hubId: string | undefined;
  readonly collectionIds: readonly string[];
  readonly wanted: boolean;
}

/** Whether a line names a release or a sprint, so their names are wanted. */
export const namesPlan = (line: string): boolean => /\bplan\.(release|sprint)\b/u.test(line);

export function planNames(scope: () => PlanNamesScope) {
  let releases = $state<ReleaseList['data'] | undefined>(undefined);
  const sprints = $state<Record<string, SprintList['data']>>({});
  $effect(() => {
    const { hubId, collectionIds, wanted } = scope();
    if (!wanted || !hubId) return;
    return untrack(() => {
      const stops = [
        engine.subscribe<ReleaseList>({ path: releasesPath(hubId) }, (next) => {
          if (next.status === 'ready') releases = next.data.data;
          if (next.status === 'failed') releases ??= [];
        }),
        ...collectionIds.map((id) => engine.subscribe<SprintList>({ path: sprintsPath(id) }, (next) => {
          if (next.status === 'ready') sprints[id] = next.data.data;
          if (next.status === 'failed') sprints[id] ??= [];
        })),
      ];
      return () => stops.forEach((stop) => stop());
    });
  });
  const allSprints = () => Object.values(sprints).flat();
  const same = (a: string, b: string) => a.toLowerCase() === b.toLowerCase();
  return {
    release: (name: string) => releases?.find((each) => same(each.name, name))?.id,
    sprint: (name: string) => allSprints().find((each) => same(each.name, name))?.id,
    releaseName: (id: string | null | undefined) => (id ? releases?.find((each) => each.id === id)?.name : undefined),
    sprintName: (id: string | null | undefined) => (id ? allSprints().find((each) => each.id === id)?.name : undefined),
    /** Whether the names a line asks for are read, so none is sent as typed. */
    isReady: (line: string) =>
      (!/\bplan\.release\b/u.test(line) || releases !== undefined) && (!/\bplan\.sprint\b/u.test(line) || Object.keys(sprints).length > 0 || scope().collectionIds.length === 0),
  };
}
