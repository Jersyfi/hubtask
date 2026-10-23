// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// What the palette offers wherever the reader is (UC-WRK-27 check 2, P-05): the frame's actions
// only where the frame's own controls are drawn, every settings row once with the screen it opens,
// and the places opened last.

import assert from 'node:assert/strict';
import { test } from 'node:test';

import type { Container } from '@hubtask/sync-engine';

import { SETTING_ROWS, frameActionOffers, recentOffers, recentPath, settingOffers, type Doing } from './registry.ts';

const t = (code: string, params?: Record<string, string>) => (params ? `${code} ${JSON.stringify(params)}` : code);

const HOUSE = { id: 'h1', type: 'HUB', parent_id: null, name: 'House' } as unknown as Container;
const ARCHIVED = { ...HOUSE, archived_at: '2026-10-01T00:00:00Z' } as unknown as Container;

const OWNER: Doing = { hubCreation: 'shared', hub: HOUSE, mayShapeHub: true, isOnHubPage: false, mayCapture: true };

const ids = (doing: Doing) => frameActionOffers(doing, t).map((offer) => offer.id);

test('an owner in a collection is offered capture, a collection in its hub and a hub', () => {
  assert.deepEqual(ids(OWNER), ['frame:capture', 'frame:create-collection', 'frame:create-hub']);
  const collection = frameActionOffers(OWNER, t).find((offer) => offer.id === 'frame:create-collection');
  assert.deepEqual(collection?.target, { kind: 'create', type: 'COLLECTION', parentId: 'h1' });
  assert.equal(collection?.label, 'app.palette.create_collection_in {"hub":"House"}');
});

test('a member, whose role shapes nothing, is offered no collection and only a private hub', () => {
  const member: Doing = { ...OWNER, hubCreation: 'private', mayShapeHub: false };
  assert.deepEqual(ids(member), ['frame:capture', 'frame:create-hub']);
  const hub = frameActionOffers(member, t).find((offer) => offer.id === 'frame:create-hub');
  assert.equal(hub?.label, 'app.workspace.create_private_hub');
  assert.deepEqual(hub?.target, { kind: 'create', type: 'HUB', isPrivateOnly: true });
});

test('what is refused or not yet known is absent, not offered', () => {
  assert.deepEqual(ids({ hubCreation: undefined, mayShapeHub: false, isOnHubPage: false, mayCapture: false }), []);
  assert.deepEqual(ids({ ...OWNER, hubCreation: 'none', mayCapture: false }), ['frame:create-collection']);
});

test('the hub\'s own page offers its collection itself, and an archived hub takes none', () => {
  assert.ok(!ids({ ...OWNER, isOnHubPage: true }).includes('frame:create-collection'));
  assert.ok(!ids({ ...OWNER, hub: ARCHIVED }).includes('frame:create-collection'));
});

test('every action of making carries the words for making, and capture its key', () => {
  for (const offer of frameActionOffers(OWNER, t)) assert.equal(offer.keywords, 'app.palette.words_create');
  assert.equal(frameActionOffers(OWNER, t)[0]?.shortcut, 'capture');
});

test('every settings row opens its screen at its own row, said with the screen it is on', () => {
  const offers = settingOffers({ categories: [] }, t);
  assert.equal(offers.length, SETTING_ROWS.length);
  assert.equal(new Set(offers.map((offer) => offer.id)).size, offers.length);
  const language = offers.find((offer) => offer.id === 'language');
  assert.deepEqual(language?.target, { kind: 'setting', path: '/profile', row: 'language' });
  assert.equal(language?.label, 'app.profile.language');
  assert.equal(language?.context, 'app.profile.title');
  assert.deepEqual(offers.find((offer) => offer.id === 'theme')?.target, { kind: 'setting', path: '/profile/appearance', row: 'theme' });
  assert.deepEqual(offers.find((offer) => offer.id === 'password')?.target, { kind: 'setting', path: '/profile/security', row: 'password' });
  for (const offer of offers) assert.equal(offer.kind, 'setting');
});

test('each notification category is a row of its own, landing on its line of the table', () => {
  const offers = settingOffers({ categories: ['ASSIGNED', 'MENTIONED'] }, t);
  const assigned = offers.find((offer) => offer.id === 'notify-ASSIGNED');
  assert.equal(assigned?.label, 'app.profile.category_ASSIGNED');
  assert.deepEqual(assigned?.target, { kind: 'setting', path: '/profile/notifications', row: 'notify-ASSIGNED' });
  assert.equal(offers.length, SETTING_ROWS.length + 2);
});

test('a place opened last opens again at its own address', () => {
  assert.equal(recentPath({ kind: 'item', id: 'i1', title: 'Milk' }), '/items/i1');
  assert.equal(recentPath({ kind: 'hub', id: 'h1', title: 'House' }), '/hubs/h1');
  assert.equal(recentPath({ kind: 'collection', id: 'c1', title: 'Kitchen' }), '/collections/c1');
  assert.deepEqual(recentOffers([{ kind: 'collection', id: 'c1', title: 'Kitchen' }]), [
    { kind: 'recent', id: 'collection:c1', label: 'Kitchen', target: { kind: 'route', path: '/collections/c1' } },
  ]);
});
