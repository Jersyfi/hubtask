// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { Capabilities, NotificationPreference } from '@hubtask/sync-engine';

import {
  categoriesOf,
  channelsOf,
  clearedOr,
  isAlwaysOn,
  knownZones,
  localesOf,
  preferenceFor,
} from './preferences.ts';

const manifest = {
  supported_locales: [
    { locale: 'en', direction: 'ltr' },
    { locale: 'ar', direction: 'rtl' },
    { locale: 'de' },
  ],
  notification_categories: ['ITEM_ASSIGNED', 'INVITATION', 'SOMETHING_NEWER'],
  notification_channels: ['EMAIL'],
} as unknown as Capabilities;

test('clearing a preference sends null, not an empty string', () => {
  // "No preference" and "a value that happens to be empty" are different statements, and the
  // schema types the three fields as nullable for exactly that reason.
  assert.equal(clearedOr(''), null);
  assert.equal(clearedOr('   '), null);
  assert.equal(clearedOr(' Europe/Berlin '), 'Europe/Berlin');
});

test('the locales are the installation’s, direction included', () => {
  assert.deepEqual(localesOf(manifest), [
    { locale: 'en', direction: 'ltr' },
    { locale: 'ar', direction: 'rtl' },
    // A locale that states no direction is left to right, which is the honest default rather than
    // a guess: the manifest would say so if it were not.
    { locale: 'de', direction: 'ltr' },
  ]);
  assert.deepEqual(localesOf(undefined), []);
});

test('the categories and channels are the installation’s, unknown ones included', () => {
  // A category this version has no phrase for still renders, because `t` humanises a code it has
  // never met — which is why nothing here filters the list down to what it recognises.
  assert.deepEqual(categoriesOf(manifest), ['ITEM_ASSIGNED', 'INVITATION', 'SOMETHING_NEWER']);
  assert.deepEqual(channelsOf(manifest), ['EMAIL']);
  // An installation that reports no channels still delivers on one, and a form with no columns
  // would be a form nobody can use.
  assert.deepEqual(channelsOf(undefined), ['EMAIL']);
  assert.deepEqual(categoriesOf(undefined), []);
});

test('the invitation is the one nothing switches off', () => {
  // A switch that could be moved and would not be obeyed is worse than one that says why not.
  assert.equal(isAlwaysOn('INVITATION'), true);
  assert.equal(isAlwaysOn('ITEM_ASSIGNED'), false);
});

test('a row is found by its pair, and a missing one is missing rather than invented', () => {
  const rows = [
    { category: 'ITEM_ASSIGNED', channel: 'EMAIL', enabled: true, include_title: false, is_default: true },
  ] as NotificationPreference[];

  assert.equal(preferenceFor(rows, 'ITEM_ASSIGNED', 'EMAIL')?.is_default, true);
  assert.equal(preferenceFor(rows, 'INVITATION', 'EMAIL'), undefined);
});

test('the zones come from the platform, or not at all', () => {
  // A list compiled into a client is a list that is wrong the next time a country moves its clocks.
  const zones = knownZones();
  if (zones.length > 0) {
    assert.ok(zones.includes('Europe/Berlin'), 'a platform that answers should know Berlin');
    // Deliberately not asserting that `UTC` is among them: this platform's list has 418 zones and
    // none of them is spelled that way. What the field needs is the platform's own vocabulary, not
    // a name this client expects to find in it.
    assert.ok(zones.length > 100, 'a real zone database rather than a handful');
  }
});
