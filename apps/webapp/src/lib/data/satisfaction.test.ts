// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { ratingLinkOf, ratingPath } from './satisfaction.ts';

// UC-SVC-07 check 6: the rating page reads its token and the chosen score from the fragment, and
// nothing else.
test('a rating link keeps its token and score in the fragment', () => {
  assert.deepEqual(ratingLinkOf('#hbt_rate_abc_DEF-1.4'), { token: 'hbt_rate_abc_DEF-1', score: 4 });
  assert.deepEqual(ratingLinkOf('#hbt_rate_abc_DEF-1'), { token: 'hbt_rate_abc_DEF-1', score: undefined });
  assert.deepEqual(ratingLinkOf('#hbt_rate_abc.9'), { token: 'hbt_rate_abc', score: undefined });
  assert.deepEqual(ratingLinkOf('#hbt_form_abc.4'), { token: '', score: undefined });
  assert.deepEqual(ratingLinkOf('#hbt_rate_a<b.4'), { token: '', score: undefined });
  assert.equal(ratingPath('hbt_rate_a'), '/public/satisfaction/hbt_rate_a');
});
