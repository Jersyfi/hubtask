// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Which release a tab should be on. Every API answer names the server's build (api-guidelines.md
// §8); a tab of an older release keeps working against the newer server (expand/contract), and
// moves to the newer bundle by one full page load at the next navigation, where nothing on screen
// is lost that leaving the screen would not lose anyway. Several answers in a row must agree, so
// a rollout still half done does not bounce the tab between pods, and a build is loaded for at
// most once per tab, so a release that serves an older document cannot loop.

import { currentEntry } from './chunkfailure.ts';

export const RELEASE_STREAK = 3;

export interface BuildsHeard {
  readonly other: string | null;
  readonly streak: number;
}

/** Folds one answer's build into what was heard. Only an unbroken run of one build counts. */
export function heard(state: BuildsHeard, build: string): BuildsHeard {
  return build === state.other ? { other: build, streak: state.streak + 1 } : { other: build, streak: 1 };
}

/** The build to load at the next navigation, or null. */
export function releaseToLoad(own: string | null, state: BuildsHeard, loadedFor: readonly string[]): string | null {
  if (own === null || state.other === null || state.other === own) return null;
  if (state.streak < RELEASE_STREAK || loadedFor.includes(state.other)) return null;
  return state.other;
}

const LOADED_KEY = 'hubtask.release-loaded-for';
let state: BuildsHeard = { other: null, streak: 0 };

/** The transport's `onServerBuild`. */
export function noteServerBuild(build: string): void {
  state = heard(state, build);
}

function loadedFor(): string[] {
  try {
    const kept: unknown = JSON.parse(sessionStorage.getItem(LOADED_KEY) ?? '[]');
    return Array.isArray(kept) ? kept.filter((value): value is string => typeof value === 'string') : [];
  } catch {
    return [];
  }
}

/**
 * The router's `fullLoad`: loads `path` from the server when a newer release is due, and answers
 * whether it did. Without the session mark nothing is loaded, since nothing would stop a loop.
 */
export function loadReleaseAt(path: string): boolean {
  const release = releaseToLoad(currentEntry(), state, loadedFor());
  if (release === null) return false;
  try {
    sessionStorage.setItem(LOADED_KEY, JSON.stringify([...loadedFor(), release]));
  } catch {
    return false;
  }
  location.assign(path);
  return true;
}
