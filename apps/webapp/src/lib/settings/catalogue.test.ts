// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The settings search over the catalogue (configuration/ui.md A7): a setting is found by its name
// and its key, and listed at every level the reader reaches - and at no other.

import assert from 'node:assert/strict';
import { test } from 'node:test';

import type { SettingDefinition } from '@hubtask/sync-engine';

import { catalogueOffers, labelCode, screenOf } from './catalogue.ts';

const t = (code: string) => code;

const define = (key: string, levels: SettingDefinition['levels'], kind: SettingDefinition['kind'] = 'COUNT'): SettingDefinition => ({
  key,
  kind,
  levels,
  lockable: true,
  restrictable: false,
  writers: levels.map((level) => ({ level, permission: 'WORKSPACE_SETTINGS' })),
});

const CATALOGUE = [
  define('sign_in.min_length', ['INSTANCE', 'PLAN', 'WORKSPACE']),
  define('quotas.items', ['INSTANCE', 'PLAN']),
  define('module.planning', ['INSTANCE', 'PLAN', 'WORKSPACE', 'HUB'], 'FLAG'),
  define('structure.views', ['INSTANCE', 'WORKSPACE', 'HUB', 'COLLECTION'], 'STRUCTURE'),
  define('sign_in.rotation_from', ['INSTANCE', 'PLAN', 'WORKSPACE'], 'STRUCTURE'),
];

test('an administrator finds every workspace setting, each on the screen that draws it', () => {
  const offers = catalogueOffers(CATALOGUE, { isAdministrationReachable: true, isInstanceReachable: false }, t);
  assert.deepEqual(
    offers.map((offer) => offer.id),
    ['WORKSPACE:sign_in.min_length', 'WORKSPACE:module.planning', 'WORKSPACE:structure.views'],
  );
  const planning = offers.find((offer) => offer.id === 'WORKSPACE:module.planning');
  assert.deepEqual(planning?.target, { kind: 'setting', path: '/administration/modules', row: 'module.planning' });
  assert.equal(planning?.label, 'app.modules.name_planning');
  assert.equal(planning?.keywords, 'module.planning');
});

test('an operator finds a setting at the installation and at the plans as well', () => {
  const offers = catalogueOffers(CATALOGUE, { isAdministrationReachable: true, isInstanceReachable: true }, t);
  const lengths = offers.filter((offer) => offer.target.kind === 'setting' && offer.target.row === 'sign_in.min_length');
  assert.deepEqual(
    lengths.map((offer) => offer.context),
    ['app.instance.settings', 'app.instance.plans', 'app.admin.sign_in'],
  );
  assert.ok(offers.some((offer) => offer.id === 'PLAN:quotas.items'));
});

test('a member, who reaches no settings screen, is offered no catalogue row', () => {
  assert.deepEqual(catalogueOffers(CATALOGUE, { isAdministrationReachable: false, isInstanceReachable: false }, t), []);
});

test('a part of another control has no row of its own', () => {
  const offers = catalogueOffers(CATALOGUE, { isAdministrationReachable: true, isInstanceReachable: true }, t);
  assert.ok(!offers.some((offer) => offer.id.endsWith('sign_in.rotation_from')));
});

test('a name is the code its own screen draws it with', () => {
  assert.equal(labelCode('sign_in.min_length'), 'app.instance.setting.sign_in.min_length');
  assert.equal(labelCode('structure.views'), 'app.structure.setting.views');
  assert.equal(labelCode('module.service_desk'), 'app.modules.name_service_desk');
});

test('a level that does not decide a setting has no screen for it', () => {
  assert.equal(screenOf(define('quotas.items', ['INSTANCE', 'PLAN']), 'WORKSPACE'), undefined);
});
