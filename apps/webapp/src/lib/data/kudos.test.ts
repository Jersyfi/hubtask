// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { kudosOffer, namesOf, showsMilestone } from './kudos.ts';

const anna = 'anna';
const ben = 'ben';
const doneBy = (who: string | null) => ({ completion: { is_completed: true, completed_by: who } });

test('UC-WRK-29 check 5: kudos are offered on somebody else\'s finished work, received on one\'s own', () => {
  assert.equal(kudosOffer(doneBy(anna), ben, true), 'give');
  assert.equal(kudosOffer(doneBy(anna), anna, true), 'received');
  assert.equal(kudosOffer({ completion: { is_completed: false, completed_by: null } }, ben, true), 'none');
  assert.equal(kudosOffer({}, ben, true), 'none');
  // A completion that names nobody has nobody to acknowledge.
  assert.equal(kudosOffer(doneBy(null), ben, true), 'none');
  // Before the account is read, nobody is "me".
  assert.equal(kudosOffer(doneBy(anna), undefined, true), 'none');
});

test('UC-WRK-29 check 7: the celebrations switch turns kudos and milestones off together', () => {
  assert.equal(kudosOffer(doneBy(anna), ben, false), 'none');
  assert.equal(kudosOffer(doneBy(anna), anna, false), 'none');
  const reached = { reached_at: '2026-10-22T10:00:00Z' };
  assert.equal(showsMilestone(reached, true), true);
  assert.equal(showsMilestone(reached, false), false);
  assert.equal(showsMilestone(undefined, true), false);
  assert.equal(showsMilestone(null, true), false);
});

test('UC-WRK-29 check 6: the givers are named, never counted', () => {
  assert.equal(namesOf(['Ben', 'Clara', 'Dana'], 'en'), 'Ben, Clara, and Dana');
  assert.equal(namesOf(['Ben'], 'en'), 'Ben');
  assert.doesNotMatch(namesOf(['Ben', 'Clara'], 'de'), /\d/);
});
