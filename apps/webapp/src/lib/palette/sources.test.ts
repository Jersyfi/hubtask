// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// What the palette may list, per role and state (UC-WRK-27 check 2, P-05). The expectations are
// literal - which role creates is written out here, as the shared walks' fixture declares it - and
// never `itemPermitted`'s own answer, so the test cannot agree with a bug by asking the code.

import assert from 'node:assert/strict';
import { test } from 'node:test';

import type { Capabilities, Container } from '@hubtask/sync-engine';

import { collectionOffers, containerOffers, contextOf, mayCreateIn, placeOffers, type Placed, type Reach } from './sources.ts';
import { layoutsOffered } from '../entries/layouts.ts';

const ALL = { read: 'ALL', create: 'ALL', change: 'ALL', comment: 'ALL' };
const ROLES = [
  { role: 'OWNER', permissions: ['READ', 'WRITE_ITEMS', 'STRUCTURE', 'MANAGE_MEMBERS', 'AUTOMATION', 'DELETE_CONTAINER', 'READ_CONFIGURATION', 'AUDIT_READ'], item_access: ALL },
  { role: 'MEMBER', permissions: ['READ', 'WRITE_ITEMS', 'AUTOMATION'], item_access: ALL },
  { role: 'CONTRIBUTOR', permissions: ['READ', 'WRITE_ITEMS'], item_access: { read: 'ALL', create: 'ALL', change: 'ASSIGNED', comment: 'ASSIGNED' } },
  { role: 'VIEWER', permissions: ['READ'], item_access: { read: 'ALL', create: 'NONE', change: 'NONE', comment: 'NONE' } },
  { role: 'AUDITOR', permissions: ['READ_CONFIGURATION', 'AUDIT_READ'], item_access: { read: 'NONE', create: 'NONE', change: 'NONE', comment: 'NONE' } },
];

const MANIFEST = {
  item_types: [
    { type: 'TASK', allowed_child_types: ['WORK_PACKAGE', 'ACTIVITY'] },
    { type: 'WORK_PACKAGE', allowed_child_types: ['ACTIVITY'] },
    { type: 'ACTIVITY', allowed_child_types: [] },
  ],
  roles: ROLES,
  view_layouts: ['LIST_COLLAPSED', 'LIST_EXPANDED', 'KANBAN', 'TIMELINE', 'CALENDAR'],
} as unknown as Capabilities;

const DRAWABLE = ['LIST_COLLAPSED', 'LIST_EXPANDED', 'KANBAN', 'TIMELINE'];

const t = (code: string, params?: Record<string, string>) => (params ? `${code} ${JSON.stringify(params)}` : code);

const HOUSE = { id: 'h1', type: 'HUB', parent_id: null, name: 'House' } as unknown as Container;
const OFFICE = { id: 'h2', type: 'HUB', parent_id: null, name: 'Office' } as unknown as Container;
const KITCHEN = { id: 'c1', type: 'COLLECTION', parent_id: 'h1', name: 'Kitchen' } as unknown as Container;
const OFFICE_KITCHEN = { id: 'c2', type: 'COLLECTION', parent_id: 'h2', name: 'kitchen' } as unknown as Container;
const ARCHIVED = { ...KITCHEN, id: 'c3', name: 'Last winter', archived_at: '2026-08-01T09:00:00Z' } as unknown as Container;
const UNDER_ARCHIVED = { ...KITCHEN, id: 'c4', name: 'Pantry', effective_archived: true } as unknown as Container;

const placed = (container: Container, hub?: Container): Placed => ({ container, hub });
const ids = (offers: readonly { id: string }[]) => offers.map((offer) => offer.id);

test('a role that creates may create in an active collection; one that does not, may not', () => {
  const expected: Record<string, boolean> = { OWNER: true, MEMBER: true, CONTRIBUTOR: true, VIEWER: false, AUDITOR: false };
  for (const [role, may] of Object.entries(expected)) {
    assert.equal(mayCreateIn(KITCHEN, role, MANIFEST), may, role);
  }
});

test('nothing is created while the role or the manifest is unknown, or where the reader holds nothing', () => {
  assert.equal(mayCreateIn(KITCHEN, undefined, MANIFEST), false);
  assert.equal(mayCreateIn(KITCHEN, 'OWNER', undefined), false);
  assert.equal(mayCreateIn(KITCHEN, null, MANIFEST), false);
});

test('nothing is created in a collection put aside, itself or under its hub, nor in a hub', () => {
  assert.equal(mayCreateIn(ARCHIVED, 'OWNER', MANIFEST), false);
  assert.equal(mayCreateIn(UNDER_ARCHIVED, 'OWNER', MANIFEST), false);
  assert.equal(mayCreateIn(HOUSE, 'OWNER', MANIFEST), false);
});

test('nothing is created where the installation offers no type at the top level', () => {
  const circular = { ...MANIFEST, item_types: [{ type: 'TASK', allowed_child_types: ['TASK'] }] } as unknown as Capabilities;
  assert.equal(mayCreateIn(KITCHEN, 'OWNER', circular), false);
});

test('the layouts are the reported ones this client draws, the list once', () => {
  assert.deepEqual(layoutsOffered(KITCHEN, MANIFEST, DRAWABLE), ['LIST_COLLAPSED', 'KANBAN', 'TIMELINE']);
  const outlineOnly = { ...MANIFEST, view_layouts: ['LIST_EXPANDED', 'KANBAN'] } as unknown as Capabilities;
  assert.deepEqual(layoutsOffered(KITCHEN, outlineOnly, DRAWABLE), ['LIST_EXPANDED', 'KANBAN']);
  assert.deepEqual(layoutsOffered(HOUSE, MANIFEST, DRAWABLE), []);
  assert.deepEqual(layoutsOffered(KITCHEN, undefined, DRAWABLE), []);
});

test('a member typing a collection\'s name is offered a search in it, its layouts and a new entry', () => {
  const all = [placed(KITCHEN, HOUSE)];
  const offers = collectionOffers(all[0]!, all, { role: 'MEMBER', manifest: MANIFEST, drawable: DRAWABLE, t });
  assert.deepEqual(ids(offers), ['search-in:c1', 'open-as:c1:LIST_COLLAPSED', 'open-as:c1:KANBAN', 'open-as:c1:TIMELINE', 'new-in:c1']);
});

test('an action the role refuses is absent: a viewer gets no new entry', () => {
  const all = [placed(KITCHEN, HOUSE)];
  const offers = collectionOffers(all[0]!, all, { role: 'VIEWER', manifest: MANIFEST, drawable: DRAWABLE, t });
  assert.ok(!ids(offers).includes('new-in:c1'));
  assert.ok(ids(offers).includes('search-in:c1'));
});

test('and nobody gets one while the role is still being read', () => {
  const all = [placed(KITCHEN, HOUSE)];
  const offers = collectionOffers(all[0]!, all, { role: undefined, manifest: MANIFEST, drawable: DRAWABLE, t });
  assert.ok(!ids(offers).includes('new-in:c1'));
});

test('the search names the collection by its id, which two collections of one name cannot share', () => {
  const all = [placed(KITCHEN, HOUSE)];
  const [search] = collectionOffers(all[0]!, all, { role: 'MEMBER', manifest: MANIFEST, drawable: DRAWABLE, t });
  assert.deepEqual(search?.target, { kind: 'search', line: 'in:c1' });
});

test('two collections named alike show their hubs; a name nobody shares shows none', () => {
  const all = [placed(KITCHEN, HOUSE), placed(OFFICE_KITCHEN, OFFICE), placed(HOUSE), placed(OFFICE)];
  assert.equal(contextOf(all[0]!, all), 'House');
  assert.equal(contextOf(all[1]!, all), 'Office');
  assert.equal(contextOf(all[2]!, all), undefined);
  assert.equal(contextOf(placed(KITCHEN, HOUSE), [placed(KITCHEN, HOUSE)]), undefined);
});

test('a container put aside itself is not listed', () => {
  assert.deepEqual(ids(containerOffers([placed(KITCHEN, HOUSE), placed(ARCHIVED, HOUSE)])), ['c1']);
});

const MEMBER_REACH: Reach = {
  hasJumble: true,
  isAdministrationReachable: false,
  isInstanceReachable: false,
  isSingleWorkspace: true,
  isPrivateHubsReadable: false,
  hasAnyPrivateHub: false,
  hasSeveralPeople: false,
};

test('a member reaches the places and their own settings, and no administration or installation', () => {
  const offers = placeOffers(MEMBER_REACH, t);
  assert.ok(ids(offers).includes('jumble'));
  assert.ok(ids(offers).includes('profile:security'));
  assert.ok(ids(offers).includes('trash'));
  assert.ok(!offers.some((offer) => offer.area === 'administration'));
  assert.ok(!offers.some((offer) => offer.area === 'instance'));
});

test('a reader without READ on the workspace has no jumble, and not before it is known', () => {
  assert.ok(!ids(placeOffers({ ...MEMBER_REACH, hasJumble: false }, t)).includes('jumble'));
  assert.ok(!ids(placeOffers({ ...MEMBER_REACH, hasJumble: undefined }, t)).includes('jumble'));
});

test('an administrator reaches the administration\'s pages, once each, with their area', () => {
  const offers = placeOffers({ ...MEMBER_REACH, isAdministrationReachable: true }, t);
  const admin = offers.filter((offer) => offer.area === 'administration');
  assert.ok(admin.some((offer) => offer.id === 'administration:people'));
  assert.ok(!admin.some((offer) => offer.id === 'administration:private-hubs'), 'private hubs only where the frame draws them');
  const paths = offers.flatMap((offer) => (offer.target.kind === 'route' ? [offer.target.path] : []));
  assert.equal(new Set(paths).size, paths.length, 'a page reached two ways is listed once');
});

test('the private hubs row follows the frame\'s answer', () => {
  const offers = placeOffers({ ...MEMBER_REACH, isAdministrationReachable: true, isPrivateHubsReadable: true, hasSeveralPeople: true }, t);
  assert.ok(ids(offers).includes('administration:private-hubs'));
});

test('an operator reaches the installation; on a multi-workspace installation its plans too', () => {
  const single = placeOffers({ ...MEMBER_REACH, isInstanceReachable: true }, t);
  assert.ok(ids(single).includes('instance:workspaces'));
  assert.ok(!ids(single).includes('instance:plans'));
  const multi = placeOffers({ ...MEMBER_REACH, isInstanceReachable: true, isSingleWorkspace: false }, t);
  assert.ok(ids(multi).includes('instance:plans'));
});

test('the account\'s verbs are rows that perform them', () => {
  const signOut = placeOffers(MEMBER_REACH, t).find((offer) => offer.id === 'sign-out');
  assert.deepEqual(signOut?.target, { kind: 'account', action: 'sign-out' });
});
