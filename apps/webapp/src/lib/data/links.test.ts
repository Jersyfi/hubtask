// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// UC-PLN-01 on the client: check 1 (each end names the link from its own side), check 4 (a
// reference out of sight reads as nothing, and the text holds no title), check 5 (an archived type
// draws no new link), check 6 (a dependency points from the entry waited for to the waiting one,
// early when it starts before the other is due) and check 7 (a text's mentions and cards, read
// the way the server keeps them).

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { EntryLink, EntryLinks, LinkType, ReferenceTarget } from '@hubtask/sync-engine';

import {
  dependenciesAmong, drawableTypes, groupLinks, inserted, referenceText, referencesIn, sideName, targetLabel,
} from './links.ts';

const t = (code: string) => `<${code}>`;

const shipped = (key: string, position: number, waits: LinkType['waits'] = 'NONE'): LinkType => ({
  key, system: true, name: null, inverse_name: null, direction: 'DIRECTED', cardinality: 'MANY',
  cycles: 'FORBIDDEN', waits, position, archived: false, version: 0,
});

const types: LinkType[] = [
  shipped('blocks', 0, 'TARGET'),
  shipped('depends_on', 1, 'SOURCE'),
  { ...shipped('relates', 7), system: false, name: 'relates to', direction: 'SYMMETRIC', cycles: 'ALLOWED', version: 1 },
  { ...shipped('tests', 6), system: false, name: 'tests', inverse_name: 'is tested by', version: 1 },
  { ...shipped('old', 8), system: false, name: 'old', inverse_name: 'old by', archived: true, version: 2 },
];

const A = '00000000-0000-4000-8000-00000000000a';
const B = '00000000-0000-4000-8000-00000000000b';
const C = '00000000-0000-4000-8000-00000000000c';

test('a shipped link is worded by the catalogue from each end, its own by its names (check 1)', () => {
  assert.equal(sideName(types[0], 'blocks', true, t), '<app.links.out.blocks>');
  assert.equal(sideName(types[0], 'blocks', false, t), '<app.links.in.blocks>');
  assert.equal(sideName(types[3], 'tests', true, t), 'tests');
  assert.equal(sideName(types[3], 'tests', false, t), 'is tested by');
  assert.equal(sideName(types[2], 'relates', false, t), 'relates to', 'a symmetric type reads the same from both ends');
  assert.equal(sideName(undefined, 'implements', false, t), '<app.links.in.implements>', 'a registry not read yet');
});

test('an archived type draws no new link; the rest in the registry order (check 5)', () => {
  assert.deepEqual(drawableTypes(types).map((type) => type.key), ['blocks', 'depends_on', 'tests', 'relates']);
});

test('the links are grouped by what they are called from this entry (check 1)', () => {
  const other = (id: string, available = true): ReferenceTarget => ({ kind: 'ENTRY', id, anchor: null, available });
  const links: EntryLinks['links'] = [
    { id: 'l1', type: 'blocks', source_id: A, target_id: B, created_at: '', outward: true, other: other(B) },
    { id: 'l2', type: 'blocks', source_id: C, target_id: A, created_at: '', outward: false, other: other(C, false) },
    { id: 'l3', type: 'blocks', source_id: A, target_id: C, created_at: '', outward: true, other: other(C) },
  ];
  const groups = groupLinks(links, types, t);
  assert.deepEqual(groups.map((group) => [group.name, group.rows.map((row) => row.id)]), [
    ['<app.links.out.blocks>', ['l1', 'l3']],
    ['<app.links.in.blocks>', ['l2']],
  ]);
});

test('a dependency points from the entry waited for to the waiting one, early when it starts first (check 6)', () => {
  const links: EntryLink[] = [
    // B depends on A: B waits.
    { id: 'd1', type: 'depends_on', source_id: B, target_id: A, created_at: '', waits: 'SOURCE' },
    // A blocks C: C waits.
    { id: 'd2', type: 'blocks', source_id: A, target_id: C, created_at: '', waits: 'TARGET' },
    // Not a dependency.
    { id: 'n1', type: 'relates', source_id: A, target_id: B, created_at: '', waits: 'NONE' },
    // An end not on the timeline.
    { id: 'd3', type: 'blocks', source_id: A, target_id: '00000000-0000-4000-8000-0000000000ff', created_at: '', waits: 'TARGET' },
  ];
  const dated = [
    { id: A, start: '2026-11-01', due: '2026-11-10' },
    { id: B, start: '2026-11-05', due: '2026-11-20' },
    { id: C, start: '2026-11-10', due: '2026-11-12' },
  ];
  assert.deepEqual(dependenciesAmong(links, dated), [
    { id: 'd1', waitedFor: A, waiting: B, isEarly: true },
    { id: 'd2', waitedFor: A, waiting: C, isEarly: false },
  ]);
  assert.equal(dependenciesAmong(links.slice(0, 1), [{ id: A }, { id: B, due: '2026-11-01' }])[0]?.isEarly, false, 'nothing due, nothing early');
});

test('a text names its references once each, a card over a mention, a view embedded (check 7)', () => {
  const text = `See [](hubtask:entry/${A}) and ![](hubtask:entry/${A}), the board ![](hubtask:view/${B}#done), ` +
    `[x](hubtask:spreadsheet/${C}) and bare hubtask:entry/${C}.`;
  assert.deepEqual(referencesIn(text), [
    { reference: { kind: 'ENTRY', id: A }, strength: 'CARD' },
    { reference: { kind: 'VIEW', id: B, anchor: 'done' }, strength: 'EMBED' },
  ]);
  assert.deepEqual(referencesIn('no address here'), []);
});

test('a mention is written as an address with no title, and lands between words (check 4)', () => {
  assert.equal(referenceText(A, 'MENTION'), `[](hubtask:entry/${A})`);
  assert.equal(referenceText(A, 'CARD'), `![](hubtask:entry/${A})`);
  const at = inserted('waits forthe design', 9, 9, '[](x)');
  assert.equal(at.text, 'waits for [](x) the design');
  assert.equal(at.caret, 'waits for [](x) '.length);
  assert.deepEqual(inserted('', 0, 0, '[](x)'), { text: '[](x)', caret: 5 });
});

test('a reference out of sight reads as nothing - never a title (check 4)', () => {
  assert.equal(targetLabel({ kind: 'ENTRY', id: A, anchor: null, available: false, title: 'leaked' }), undefined);
  assert.equal(targetLabel({ kind: 'ENTRY', id: A, anchor: null, available: true, title: 'Approve', key: 'des-4' }), 'des-4 Approve');
  assert.equal(targetLabel(undefined), undefined);
});
