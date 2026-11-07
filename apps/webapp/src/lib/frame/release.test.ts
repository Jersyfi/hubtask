// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A tab of the older release moves to the newer one by one full page load at a navigation - once
// the API has named the other build on several answers in a row, so a rollout still half done does
// not bounce a tab between pods, and at most once per build, so it cannot loop.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { heard, releaseToLoad, RELEASE_STREAK, type BuildsHeard } from './release.ts';

const own = '/assets/index-OLD.js';
const hear = (builds: readonly string[]): BuildsHeard =>
  builds.reduce(heard, { other: null, streak: 0 } as BuildsHeard);

test('one answer of another build is not yet a release to load', () => {
  assert.equal(releaseToLoad(own, hear(['/assets/index-NEW.js']), []), null);
});

test('the same other build on several answers in a row is loaded at the next navigation', () => {
  const builds = Array.from({ length: RELEASE_STREAK }, () => '/assets/index-NEW.js');
  assert.equal(releaseToLoad(own, hear(builds), []), '/assets/index-NEW.js');
});

test('answers alternating between two pods never add up to a streak', () => {
  const builds = ['/assets/index-NEW.js', own, '/assets/index-NEW.js', own, '/assets/index-NEW.js'];
  assert.equal(releaseToLoad(own, hear(builds), []), null);
});

test('a build already loaded for in this tab is not loaded again', () => {
  const builds = Array.from({ length: RELEASE_STREAK }, () => '/assets/index-NEW.js');
  assert.equal(releaseToLoad(own, hear(builds), ['/assets/index-NEW.js']), null);
});

test('a tab that does not know its own build loads nothing', () => {
  const builds = Array.from({ length: RELEASE_STREAK }, () => '/assets/index-NEW.js');
  assert.equal(releaseToLoad(null, hear(builds), []), null);
});
