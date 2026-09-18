// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { candidateKindOf, candidateProblem, documentOf, draftOf, moved, type Candidate } from './containerpolicies.ts';

const AMELIE: Candidate = { kind: 'ACCOUNT', id: '22222222-0000-4000-8000-00000000000a' };
const JONAS: Candidate = { kind: 'ACCOUNT', id: '22222222-0000-4000-8000-00000000000b' };
const FITTERS: Candidate = { kind: 'GROUP', id: '33333333-0000-4000-8000-00000000000f' };

test('the form is filled from what the collection holds, and empty where it holds nothing', () => {
  assert.deepEqual(draftOf(undefined), { completion: '', strategy: '', candidates: [], enabled: true });
  assert.deepEqual(draftOf({}), { completion: '', strategy: '', candidates: [], enabled: true });
  assert.deepEqual(
    draftOf({ completion_policy: 'ROLLUP', auto_assign: { strategy: 'ROUND_ROBIN', candidates: [AMELIE, JONAS], enabled: false } }),
    { completion: 'ROLLUP', strategy: 'ROUND_ROBIN', candidates: [AMELIE, JONAS], enabled: false },
  );
});

test('the document is written whole: every key with a value, and null for no assignment (a PUT)', () => {
  assert.deepEqual(documentOf({ completion: '', strategy: '', candidates: [], enabled: true }), { auto_assign: null });
  assert.deepEqual(
    documentOf({ completion: 'ROLLUP', strategy: 'FIXED', candidates: [AMELIE], enabled: true }),
    { completion_policy: 'ROLLUP', auto_assign: { strategy: 'FIXED', candidates: [AMELIE], enabled: true } },
  );
});

test('the three rules are predicted in the server\'s own codes', () => {
  assert.equal(candidateProblem({ completion: '', strategy: '', candidates: [], enabled: true }), undefined);
  assert.deepEqual(
    candidateProblem({ completion: '', strategy: 'ROUND_ROBIN', candidates: [], enabled: true }),
    { code: 'containers.auto_assign_candidates_required' },
  );
  assert.deepEqual(
    candidateProblem({ completion: '', strategy: 'FIXED', candidates: [AMELIE, JONAS], enabled: true }),
    { code: 'containers.auto_assign_single_candidate_required', params: { count: '2' } },
  );
  assert.deepEqual(
    candidateProblem({ completion: '', strategy: 'RANDOM_GROUP_MEMBER', candidates: [AMELIE], enabled: true }),
    { code: 'containers.auto_assign_candidate_kind_invalid', params: { strategy: 'RANDOM_GROUP_MEMBER', kind: 'ACCOUNT' } },
  );
  assert.deepEqual(
    candidateProblem({ completion: '', strategy: 'LEAST_LOADED', candidates: [FITTERS], enabled: true }),
    { code: 'containers.auto_assign_candidate_kind_invalid', params: { strategy: 'LEAST_LOADED', kind: 'GROUP' } },
  );
  assert.equal(candidateProblem({ completion: '', strategy: 'RANDOM_GROUP_MEMBER', candidates: [FITTERS], enabled: true }), undefined);
  assert.equal(candidateProblem({ completion: '', strategy: 'FIXED', candidates: [AMELIE], enabled: true }), undefined);
});

test('groups for RANDOM_GROUP_MEMBER, accounts for everything else', () => {
  assert.equal(candidateKindOf('RANDOM_GROUP_MEMBER'), 'GROUP');
  for (const strategy of ['FIXED', 'RANDOM_MEMBER', 'ROUND_ROBIN', 'LEAST_LOADED']) assert.equal(candidateKindOf(strategy), 'ACCOUNT');
});

test('the order is the round robin\'s, and a move past either end changes nothing', () => {
  assert.deepEqual(moved([AMELIE, JONAS], 1, -1), [JONAS, AMELIE]);
  assert.deepEqual(moved([AMELIE, JONAS], 0, 1), [JONAS, AMELIE]);
  assert.deepEqual(moved([AMELIE, JONAS], 0, -1), [AMELIE, JONAS]);
  assert.deepEqual(moved([AMELIE, JONAS], 1, 1), [AMELIE, JONAS]);
});
