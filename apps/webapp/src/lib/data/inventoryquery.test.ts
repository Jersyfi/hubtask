// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import type { CiClass, ConfigurationItem } from '@hubtask/sync-engine';

import {
  DEFAULT_LINE, classTree, groupOfLine, highestProtection, holdsAttributes, lineOfGroup, narrowed, readLine, soleValue, termsOf, valueCounts, withKey,
  type NarrowContext,
} from './inventoryquery.ts';

const cls = (key: string, lineage: string[]): CiClass => ({
  key, parent: lineage[1] ?? '', system: true, attributes: [], all_attributes: [], identifier_rules: [], required_relations: [],
  staleness_days: 60, lineage, version: 1,
});
const classes = [
  cls('ci', ['ci']),
  cls('hardware', ['hardware', 'ci']),
  cls('server', ['server', 'hardware', 'ci']),
  cls('laptop', ['laptop', 'hardware', 'ci']),
  cls('service', ['service', 'ci']),
  cls('application_service', ['application_service', 'service', 'ci']),
  cls('location', ['location', 'ci']),
  cls('room', ['room', 'location', 'ci']),
];
const ME = 'acc-me';
const ci = (id: string, klass: string, extra: Partial<ConfigurationItem> = {}): ConfigurationItem => ({
  id, hub_id: 'h', class: klass, name: id, stage: 'IN_USE', status: 'OK', attributes: {}, provenance: {}, protection: {}, quantity: 1,
  created_at: '2026-10-01T00:00:00Z', updated_at: '2026-10-01T00:00:00Z', version: 1, ...extra,
});
const cis = [
  ci('Portal-API', 'application_service', { owner_group_id: 'grp-platform' }),
  ci('db-01.hh.intern', 'server', { owner_account_id: ME, location_id: 'rz-hh' }),
  ci('app-01.hh.intern', 'server', { stage: 'MAINTENANCE', owner_group_id: 'grp-platform', location_id: 'rz-hh' }),
  ci('NB-0422', 'laptop', { location_id: 'office' }),
  ci('NB-0233', 'laptop', { stage: 'DISPOSED', owner_account_id: 'acc-other' }),
  ci('old-merged', 'server', { merged_into: 'db-01.hh.intern' }),
];
const context: NarrowContext = { classes, me: ME, tags: new Map([['NB-0422', 'NB-0422'], ['NB-0233', 'nb-0233']]) };
const names = (line: string) => {
  const read = readLine(line);
  assert.ok(read.ok, `${line}: ${JSON.stringify(read)}`);
  return narrowed(cis, read.filter, context).map((each) => each.id);
};

test('the inventory line narrows by object type with the types beneath it, compared lowered', () => {
  assert.deepEqual(names('asset.class: hardware'), ['db-01.hh.intern', 'app-01.hh.intern', 'NB-0422', 'NB-0233']);
  assert.deepEqual(names('asset.class: Server'), ['db-01.hh.intern', 'app-01.hh.intern']);
  assert.deepEqual(names('asset.class: server, laptop'), ['db-01.hh.intern', 'app-01.hh.intern', 'NB-0422', 'NB-0233']);
  assert.deepEqual(names('-asset.class: hardware'), ['Portal-API']);
  // A merged object lives on in the one it was merged into: never a row of its own.
  assert.ok(!names('').includes('old-merged'));
});

test('the inventory line narrows by stage, the default leaving the disposed out', () => {
  assert.equal(DEFAULT_LINE, '-asset.stage: disposed');
  assert.ok(!names(DEFAULT_LINE).includes('NB-0233'));
  assert.equal(names(DEFAULT_LINE).length, 4);
  assert.deepEqual(names('asset.stage: maintenance'), ['app-01.hh.intern']);
  assert.deepEqual(names('asset.stage: in_use, maintenance asset.class: server'), ['db-01.hh.intern', 'app-01.hh.intern']);
});

test('owner, location and tag take identifiers, none, me, and turn round with a minus', () => {
  assert.deepEqual(names('asset.owner: none'), ['NB-0422']);
  assert.deepEqual(names('-asset.owner: none'), ['Portal-API', 'db-01.hh.intern', 'app-01.hh.intern', 'NB-0233']);
  assert.deepEqual(names('asset.owner: me'), ['db-01.hh.intern']);
  assert.deepEqual(names('asset.owner: grp-platform, none'), ['Portal-API', 'app-01.hh.intern', 'NB-0422']);
  assert.deepEqual(names('asset.location: rz-hh'), ['db-01.hh.intern', 'app-01.hh.intern']);
  assert.deepEqual(names('asset.location: none'), ['Portal-API', 'NB-0233']);
  assert.deepEqual(names('asset.tag: nb-0422'), ['NB-0422']);
  assert.deepEqual(names('asset.tag: NB-0233'), ['NB-0233']);
  assert.deepEqual(names('asset.tag: none asset.class: laptop'), []);
});

test('free words must all be in the name; a minus word and OR are left out as the server does', () => {
  assert.deepEqual(termsOf('Portal "api gw" -old OR portal'), ['portal', 'api', 'gw']);
  assert.deepEqual(names('hh intern'), ['db-01.hh.intern', 'app-01.hh.intern']);
  assert.deepEqual(names('asset.class: server db'), ['db-01.hh.intern']);
  assert.deepEqual(names('PORTAL'), ['Portal-API']);
});

test('a line the inventory cannot read is refused with the server\'s code', () => {
  const twice = readLine('asset.class: server asset.class: laptop');
  assert.ok(!twice.ok && twice.error.code === 'query.key_twice');
  const grouped = readLine('(asset.stage: in_use OR asset.class: server)');
  assert.ok(!grouped.ok && grouped.error.code === 'query.kind_filter_grouped');
  const mixed = readLine('is: open');
  assert.ok(!mixed.ok && mixed.error.code === 'query.kinds_mixed');
  const unknown = readLine('asset.stage: broken');
  assert.ok(!unknown.ok);
});

test('values are offered with how many objects of the rest of the line hold them', () => {
  const read = readLine('asset.class: server asset.stage: in_use');
  assert.ok(read.ok);
  // The stage counted within the servers, its own condition aside.
  assert.deepEqual(valueCounts(cis, read.filter, 'asset.stage', context), [{ value: 'in_use', count: 1 }, { value: 'maintenance', count: 1 }]);
  assert.deepEqual(valueCounts(cis, read.filter, 'asset.owner', context), [{ value: ME, count: 1 }]);
  const all = readLine('');
  assert.ok(all.ok);
  assert.deepEqual(valueCounts(cis, all.filter, 'asset.location', context), [{ value: 'none', count: 2 }, { value: 'rz-hh', count: 2 }, { value: 'office', count: 1 }]);
});

test('the object-type tree counts the types beneath, hides empty types without members beneath', () => {
  const live = cis.filter((each) => !each.merged_into);
  assert.deepEqual(classTree(live, classes), [
    { key: 'hardware', depth: 0, count: 4, hasChildren: true },
    { key: 'server', depth: 1, count: 2, hasChildren: false },
    { key: 'laptop', depth: 1, count: 2, hasChildren: false },
    { key: 'service', depth: 0, count: 1, hasChildren: true },
    { key: 'application_service', depth: 1, count: 1, hasChildren: false },
  ]);
  // A type held only through one beneath it stays, with that one's count.
  assert.deepEqual(classTree([ci('r1', 'room')], classes).map((row) => [row.key, row.count]), [['location', 1], ['room', 1]]);
  assert.deepEqual(classTree([], classes), []);
});

test('a type in the tree sets the line\'s object type, the rest of the line kept', () => {
  assert.equal(withKey(DEFAULT_LINE, 'asset.class', 'server'), 'asset.class: server -asset.stage: disposed');
  assert.equal(withKey('asset.class: laptop -asset.stage: disposed nb', 'asset.class', undefined), '-asset.stage: disposed nb');
  assert.equal(soleValue('asset.class: server -asset.stage: disposed', 'asset.class'), 'server');
  assert.equal(soleValue('asset.class: server, laptop', 'asset.class'), undefined);
});

test('a saved group fills the line; a line becomes a group where a group can hold it', () => {
  const group = lineOfGroup({ classes: ['server'], stages: ['IN_USE', 'MAINTENANCE'], no_owner: true, owners: ['grp-platform'], name_has: 'hh intern', attributes: { os: 'linux' } });
  assert.equal(group.line, 'asset.class: server asset.stage: in_use, maintenance asset.owner: grp-platform, none "hh intern"');
  assert.deepEqual(group.attributes, { os: 'linux' });
  assert.ok(readLine(group.line).ok);
  assert.deepEqual(names(group.line), ['app-01.hh.intern']);

  assert.deepEqual(groupOfLine('asset.class: server asset.owner: me, none asset.tag: NB-1 portal', ME), {
    ok: true, filter: { classes: ['server'], owners: [ME], no_owner: true, tags: ['nb-1'], name_has: 'portal' },
  });
  assert.deepEqual(groupOfLine('asset.stage: in_use "api gw"', ME), { ok: true, filter: { stages: ['IN_USE'], name_has: 'api gw' } });
  assert.deepEqual(groupOfLine(DEFAULT_LINE, ME), { ok: false, code: 'app.inventory.group_negated' });
  assert.deepEqual(groupOfLine('asset.class: server two words', ME), { ok: false, code: 'app.inventory.group_words' });
  assert.deepEqual(groupOfLine('', ME), { ok: false, code: 'cmdb.group_filter_empty' });
  // A group's line saved again is the same group.
  const again = groupOfLine(lineOfGroup({ classes: ['laptop'], locations: ['office'], no_tag: true }).line, ME);
  assert.deepEqual(again, { ok: true, filter: { classes: ['laptop'], locations: ['office'], no_tag: true } });
});

test('attribute values and the highest protection are read as the server reads them', () => {
  assert.ok(holdsAttributes(ci('x', 'server', { attributes: { os: 'Linux' } }), { os: 'linux' }));
  assert.ok(!holdsAttributes(ci('x', 'server'), { os: 'linux' }));
  assert.equal(highestProtection({}), undefined);
  assert.equal(highestProtection({ c: 'NORMAL', a: 'HIGH' }), 'HIGH');
  assert.equal(highestProtection({ i: 'VERY_HIGH', c: 'HIGH' }), 'VERY_HIGH');
});
