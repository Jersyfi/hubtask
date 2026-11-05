// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// user-experience.md §3.4: a product's tour is three steps - where to create, where to find, where
// to configure -, its first visit's mark is the first of them, once per person, and only once the
// starter and the base tour are behind the person (§3.1).

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

import { PRODUCTS } from '../navigation.ts';
import { homeModuleOf, marksFirstVisit, productTourSteps, withSeen } from './producttour.ts';

const free = { baseTourOpen: false, baseTourMayStart: true, baseTourTaken: true };

test('every product with a home has its three steps, with words for each', () => {
  const catalogue = JSON.parse(readFileSync(new URL('../../../../../locales/en.json', import.meta.url), 'utf8')) as Record<string, string>;
  for (const row of PRODUCTS) {
    const steps = productTourSteps(row.module);
    assert.deepEqual(steps.map((step) => step.id), ['create', 'find', 'configure'], row.module);
    assert.equal(steps[0]?.route, row.path);
    assert.match(steps[2]?.selector ?? '', new RegExp(row.module));
    for (const step of steps) assert.ok(catalogue[`app.product_tour.${step.id}.caption`] && catalogue[`app.product_tour.${step.id}.body`]);
  }
  assert.deepEqual(productTourSteps('WORKFLOWS'), []);
});

test('the first visit is marked on a product home not yet seen, after the starter and the base tour', () => {
  assert.equal(homeModuleOf('desk-home', PRODUCTS), 'SERVICE_DESK');
  assert.equal(homeModuleOf('desk-settings', PRODUCTS), undefined);
  assert.equal(homeModuleOf('home', PRODUCTS), undefined);
  assert.equal(marksFirstVisit('SERVICE_DESK', { products_seen: [] }, free), true);
  assert.equal(marksFirstVisit('SERVICE_DESK', {}, free), true);
  assert.equal(marksFirstVisit('SERVICE_DESK', { products_seen: ['SERVICE_DESK'] }, free), false);
  assert.equal(marksFirstVisit('SERVICE_DESK', undefined, free), false);
  assert.equal(marksFirstVisit('SERVICE_DESK', {}, { ...free, baseTourOpen: true }), false);
  assert.equal(marksFirstVisit('SERVICE_DESK', {}, { ...free, baseTourMayStart: false }), false);
  assert.equal(marksFirstVisit('SERVICE_DESK', {}, { ...free, baseTourTaken: false }), false);
});

test('a product seen is added once, at the end', () => {
  assert.deepEqual(withSeen(undefined, 'TIME'), ['TIME']);
  assert.deepEqual(withSeen(['TIME'], 'GOALS'), ['TIME', 'GOALS']);
  assert.deepEqual(withSeen(['TIME', 'GOALS'], 'TIME'), ['TIME', 'GOALS']);
});
