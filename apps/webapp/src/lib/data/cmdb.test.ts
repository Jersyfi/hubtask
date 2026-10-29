// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import type { CiAttribute, CiClass, CiRelationType, ConfigurationItem } from '@hubtask/sync-engine';

import {
  changedAttributes, ciReferenceText, cisPath, classDefine, classDraft, keyIsWellFormed, classWord, fileColumns, fileFormat, guessMapping, impactByDepth, instantiable, inventoryByClass,
  missingRequired, outcomeTone, parseAttribute, percent, protectionRows, refinements, relationRows, relationWord, sectionOf,
  sideBySide, typeAllows,
} from './cmdb.ts';

const cls = (key: string, lineage: string[], extra: Partial<CiClass> = {}): CiClass => ({
  key, parent: lineage[1] ?? '', system: true, attributes: [], all_attributes: [], identifier_rules: [], required_relations: [],
  staleness_days: 60, lineage, version: 1, ...extra,
});
const classes = [
  cls('ci', ['ci'], { abstract: true }),
  cls('hardware', ['hardware', 'ci'], { abstract: true }),
  cls('server', ['server', 'computer', 'hardware', 'ci']),
  cls('virtual_machine', ['virtual_machine', 'ci']),
  cls('application_service', ['application_service', 'service', 'ci']),
  cls('room', ['room', 'location', 'ci']),
  cls('old', ['old', 'ci'], { archived: true }),
  cls('rack_server', ['rack_server', 'server', 'computer', 'hardware', 'ci'], { system: false, name: 'Rack server' }),
];
const ci = (id: string, klass: string, attributes: Record<string, unknown> = {}): ConfigurationItem => ({
  id, hub_id: 'h', class: klass, name: id, stage: 'IN_USE', status: 'OK', attributes, provenance: {}, protection: {}, quantity: 1,
  created_at: '2026-10-01T00:00:00Z', updated_at: '2026-10-01T00:00:00Z', version: 1,
});
const words = (known: Record<string, string>) => (code: string) => known[code];

test('the screen reads a hub inventory by class, page by page, merged CIs only when asked', () => {
  assert.equal(cisPath('h'), '/hubs/h/cis');
  assert.equal(cisPath('h', { class: 'server', after: 'c9' }), '/hubs/h/cis?class=server&after=c9');
  assert.equal(cisPath('h', { includeMerged: true }), '/hubs/h/cis?include_merged=true');
  assert.equal(sectionOf('health'), 'health');
  assert.equal(sectionOf('nothing'), 'inventory');
  assert.equal(sectionOf(undefined), 'inventory');
});

test('the inventory lists its CIs by class in the classes order (UC-AST-01 check 8)', () => {
  const groups = inventoryByClass([ci('b', 'virtual_machine'), ci('a', 'server'), ci('c', 'virtual_machine'), ci('z', 'unknown')], classes);
  assert.deepEqual(groups.map((group) => [group.class, group.items.map((each) => each.id)]), [
    ['server', ['a']],
    ['virtual_machine', ['b', 'c']],
    ['unknown', ['z']],
  ]);
});

test('a shipped class is named by the catalogue, an own class by its name, an unknown one by its key', () => {
  const t = words({ 'app.assets.class.server': 'Server', 'app.assets.relation.runs_on.out': 'runs on', 'app.assets.relation.runs_on.in': 'runs' });
  assert.equal(classWord('server', classes, t), 'Server');
  assert.equal(classWord('rack_server', classes, t), 'Rack server');
  assert.equal(classWord('mystery', classes, t), 'mystery');
  assert.equal(relationWord(undefined, 'runs_on', true, t), 'runs on');
  assert.equal(relationWord(undefined, 'runs_on', false, t), 'runs');
  const own: CiRelationType = { key: 'feeds', name: 'feeds', reverse_name: 'is fed by', impact: 'TARGET', pairs: [], system: false, version: 1 };
  assert.equal(relationWord(own, 'feeds', false, t), 'is fed by');
});

test('a CI is offered only classes that hold CIs, and refined only to one beneath its own', () => {
  assert.deepEqual(instantiable(classes).map((each) => each.key), ['server', 'virtual_machine', 'application_service', 'room', 'rack_server']);
  assert.deepEqual(refinements(classes, 'server').map((each) => each.key), ['rack_server']);
});

test('a typed attribute is read from its field, a wrong value refused before it is sent (UC-AST-01 check 1)', () => {
  const attr = (kind: CiAttribute['kind'], options?: string[]): CiAttribute => ({ key: 'x', kind, ...(options ? { options } : {}) });
  assert.deepEqual(parseAttribute(attr('NUMBER'), ' 16 '), { ok: true, value: 16 });
  assert.deepEqual(parseAttribute(attr('NUMBER'), '2,5'), { ok: true, value: 2.5 });
  assert.deepEqual(parseAttribute(attr('NUMBER'), 'many'), { ok: false });
  assert.deepEqual(parseAttribute(attr('BOOLEAN'), 'true'), { ok: true, value: true });
  assert.deepEqual(parseAttribute(attr('BOOLEAN'), 'yes'), { ok: false });
  assert.deepEqual(parseAttribute(attr('DATE'), '2027-03-01'), { ok: true, value: '2027-03-01' });
  assert.deepEqual(parseAttribute(attr('DATE'), '1.3.2027'), { ok: false });
  assert.deepEqual(parseAttribute(attr('CHOICE', ['linux', 'windows']), 'linux'), { ok: true, value: 'linux' });
  assert.deepEqual(parseAttribute(attr('CHOICE', ['linux']), 'bsd'), { ok: false });
  assert.deepEqual(parseAttribute(attr('TEXT'), '   '), { ok: true, value: null });
});

test('an edit sends only what changed, a cleared value as null, and names what does not parse', () => {
  const attributes: CiAttribute[] = [
    { key: 'cpu_cores', kind: 'NUMBER' }, { key: 'os', kind: 'TEXT' }, { key: 'serial', kind: 'TEXT' }, { key: 'ram_gb', kind: 'NUMBER' },
  ];
  const before = { cpu_cores: 8, os: 'Debian', serial: 'S1' };
  const out = changedAttributes(attributes, before, { cpu_cores: '16', os: 'Debian', serial: '', ram_gb: 'lots' });
  assert.deepEqual(out.changes, { cpu_cores: 16, serial: null });
  assert.deepEqual(out.invalid, ['ram_gb']);
  assert.deepEqual(changedAttributes(attributes, {}, { os: '' }).changes, {}, 'an empty field that was empty stays out');
  assert.deepEqual(missingRequired([{ key: 'serial', kind: 'TEXT', need: 'REQUIRED' }, { key: 'os', kind: 'TEXT' }], { serial: ' ' }), ['serial']);
});

test('a CI shows its relations both ways from its own end (UC-AST-01 check 5)', () => {
  const relation = (id: string, from: string, to: string) => ({
    id, type: 'runs_on', source_ci_id: from, target_ci_id: to, data_source: 'manual', first_seen: '2026-10-01T00:00:00Z', last_seen: '2026-10-02T00:00:00Z',
  });
  const rows = relationRows('vm', [relation('r2', 'app', 'vm'), relation('r1', 'vm', 'srv')]);
  assert.deepEqual(rows.map((row) => [row.id, row.outgoing, row.other]), [['r1', true, 'srv'], ['r2', false, 'app']]);
});

test('a relation is offered only where its class pairs join the two CIs (UC-AST-01 check 3)', () => {
  const hosted: CiRelationType = { key: 'hosted_on', impact: 'SOURCE', pairs: [{ from: 'virtual_machine', to: 'hardware' }], system: true, version: 1 };
  const find = (key: string) => classes.find((each) => each.key === key);
  assert.equal(typeAllows(hosted, find('virtual_machine'), find('rack_server')), true);
  assert.equal(typeAllows(hosted, find('server'), find('virtual_machine')), false);
  assert.equal(typeAllows({ ...hosted, pairs: [] }, find('server'), find('room')), true);
  assert.equal(typeAllows({ ...hosted, pairs: [], archived: true }, find('server'), find('room')), false);
});

test('the impact is grouped by depth, the nearest first (UC-AST-05 check 5)', () => {
  const reached = [
    { ci_id: 'c', class: 'business_service', name: 'Booking', stage: 'IN_USE', depth: 2 },
    { ci_id: 'a', class: 'virtual_machine', name: 'vm1', stage: 'IN_USE', depth: 1 },
    { ci_id: 'b', class: 'virtual_machine', name: 'vm2', stage: 'IN_USE', depth: 1 },
  ];
  assert.deepEqual(impactByDepth(reached).map((group) => [group.depth, group.items.map((each) => each.ci_id)]), [[1, ['a', 'b']], [2, ['c']]]);
});

test('the effective protection need stands beside the own one, raised or fixed (UC-AST-05 check 5)', () => {
  const rows = protectionRows({ c: 'NORMAL', a: 'HIGH', fixed: ['a'], reason: 'spare' }, { c: 'VERY_HIGH', i: 'HIGH', a: 'HIGH' });
  assert.deepEqual(rows.map((row) => [row.dimension, row.own, row.effective, row.raised, row.fixed]), [
    ['c', 'NORMAL', 'VERY_HIGH', true, false],
    ['i', null, 'HIGH', true, false],
    ['a', 'HIGH', 'HIGH', false, true],
  ]);
  assert.deepEqual(protectionRows(undefined, undefined).map((row) => row.effective), [null, null, null]);
});

test('duplicates stand side by side with what differs marked (UC-AST-03 check 5)', () => {
  const rows = sideBySide([ci('a', 'server', { serial: 'S1', os: 'Debian' }), ci('b', 'server', { serial: 'S1' })]);
  assert.deepEqual(rows.map((row) => [row.key, row.differ]), [['name', true], ['class', false], ['os', true], ['serial', false]]);
  assert.deepEqual(rows.find((row) => row.key === 'os')?.values, ['Debian', '']);
});

test('a run decision is drawn by its outcome, and a share as whole percent', () => {
  assert.equal(outcomeTone('CREATE'), 'success');
  assert.equal(outcomeTone('HOLD'), 'warning');
  assert.equal(outcomeTone('REFUSED'), 'danger');
  assert.equal(outcomeTone('UNCHANGED'), 'neutral');
  assert.equal(percent(0.666), 67);
  assert.equal(percent(1.2), 100);
});

test('a file is read in the format it is, and its columns offered to the mapping (UC-AST-04 check 5)', () => {
  assert.equal(fileFormat('hosts.csv', 'name,serial'), 'CSV');
  assert.equal(fileFormat('bom.json', '{"bomFormat": "CycloneDX", "specVersion": "1.5"}'), 'CYCLONEDX');
  assert.equal(fileFormat('sbom.json', '{"spdxVersion": "SPDX-2.3"}'), 'SPDX');
  assert.equal(fileFormat('hosts.json', '[{"name": "a"}]'), 'JSON');
  assert.deepEqual(fileColumns('CSV', 'Hostname;"Serial; no";OS\nweb1;S1;Debian'), ['Hostname', 'Serial; no', 'OS']);
  assert.deepEqual(fileColumns('JSON', '[{"name":"a","os":"x"},{"name":"b","ram_gb":4}]'), ['name', 'os', 'ram_gb']);
  assert.deepEqual(fileColumns('JSON', 'not json'), []);
  const guess = guessMapping(['Hostname', 'ID', 'OS', 'Colour'], [{ key: 'os', kind: 'TEXT' }]);
  assert.deepEqual(guess, { name: 'Hostname', nativeKey: 'ID', columns: { OS: 'os' } });
});

test('an own class is edited by what it declares and sent as the contract takes it (UC-AST-06 check 5)', () => {
  const own = cls('rack_server', ['rack_server', 'server', 'computer', 'hardware', 'ci'], {
    system: false, name: 'Rack server', parent: 'server', staleness_days: 30,
    attributes: [{ key: 'rack', kind: 'CHOICE', need: 'REQUIRED', identifier: false, options: ['A', 'B'] }],
    identifier_rules: [{ attributes: ['rack', 'name'], class: 'rack_server' }, { attributes: ['serial_number'], class: 'hardware' }],
    required_relations: [{ types: ['hosted_on'], outgoing: true }],
  });
  const draft = classDraft(own);
  assert.equal(draft.rules, 'rack+name', 'an inherited rule is not the class own');
  assert.equal(draft.attributes[0]?.options, 'A\nB');
  draft.attributes.push({ key: ' ', kind: 'TEXT', need: 'OPTIONAL', identifier: false, options: '' });
  draft.required.push({ types: [], outgoing: false });
  assert.deepEqual(classDefine(draft), {
    parent: 'server', name: 'Rack server', abstract: false,
    attributes: [{ key: 'rack', kind: 'CHOICE', need: 'REQUIRED', identifier: false, options: ['A', 'B'] }],
    identifier_rules: [{ attributes: ['rack', 'name'] }],
    required_relations: [{ types: ['hosted_on'], outgoing: true }],
    staleness_days: 30, archived: false,
  });
  assert.equal(classDefine({ ...classDraft(), name: ' Printer ', staleness: '' }).staleness_days, 0, 'empty inherits');
  assert.equal(keyIsWellFormed('rack_server'), true);
  assert.equal(keyIsWellFormed('Rack'), false);
  assert.equal(keyIsWellFormed('1rack'), false);
});

test('a CI is pointed at by an address the server keeps as a reference (UC-AST-01 check 9)', async () => {
  const { referencesIn } = await import('./links.ts');
  const id = '0192f000-0000-7000-8000-0000000007b1';
  assert.equal(ciReferenceText(id), `[](hubtask:ci/${id})`);
  assert.deepEqual(referencesIn(`runs on ${ciReferenceText(id, 'CARD')}`), [{ reference: { kind: 'CI', id }, strength: 'CARD' }]);
});
