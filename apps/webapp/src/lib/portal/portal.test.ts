// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// UC-SVC-05 checks 1, 2 and 4, UC-DOC-13 check 3: the portal asks the catalogue's questions by
// their kind, sends only real answers, draws a goal by how it stands, and asks articles for the
// words typed so far.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import {
  actsAsDesk, answersOf, portalTypesOf, articleText, askable, deflectionText, deskHead, deskLogoOf, goalTone, isOutsider, isOutsiderRoute,
  isTeamView, newestEverywhere, nextStep, portalStart, portalTabOf, portalTabPath, requestState, spaceOutline, teamDeskOf, wordsFind,
} from './portal.ts';

const questions = [
  { key: 'room', kind: 'SELECT', options: ['floor_1', 'floor_2'] },
  { key: 'devices', kind: 'MULTI_SELECT', options: ['laptop', 'phone'] },
  { key: 'count', kind: 'NUMBER', options: [] },
  { key: 'urgent', kind: 'BOOL', options: [] },
  { key: 'note', kind: 'TEXT', options: [] },
  { key: 'owner', kind: 'USER', options: [] },
];

test('a question the portal cannot answer is not asked', () => {
  assert.deepEqual(askable(questions).map((q) => q.key), ['room', 'devices', 'count', 'urgent', 'note']);
});

test('only real answers are sent, each in its kind', () => {
  assert.deepEqual(
    answersOf(questions, { room: 'floor_3', devices: ['laptop', 'tablet'], count: '2,5', urgent: true, note: '  ', owner: 'x' }),
    { devices: ['laptop'], count: 2.5, urgent: true },
  );
  assert.deepEqual(answersOf(questions, { room: 'floor_2', count: 'many' }), { room: 'floor_2' });
});

test('a goal is drawn by how it stands', () => {
  const now = new Date('2026-10-30T10:00:00Z');
  assert.equal(goalTone({ standing: 'RUNNING', due_at: '2026-10-30T12:00:00Z', at_risk_at: '2026-10-30T09:00:00Z' }, now), 'warning');
  assert.equal(goalTone({ standing: 'RUNNING', due_at: '2026-10-30T12:00:00Z', at_risk_at: '2026-10-30T11:00:00Z' }, now), 'info');
  assert.equal(goalTone({ standing: 'BREACHED', due_at: null, at_risk_at: null }, now), 'danger');
  assert.equal(goalTone({ standing: 'MET', due_at: null, at_risk_at: null }, now), 'success');
});

test('articles are asked for the words typed so far, never for two letters', () => {
  assert.equal(deflectionText('pr', ''), '');
  assert.equal(deflectionText('Printer  prints', 'blank\npages'), 'Printer prints blank pages');
});

// UC-SVC-05 check 8: the portal's head is the desk's own name and description, the portal's own
// words standing in for what the desk leaves empty or before the catalogue answers.
test('the head is the desk the portal is for', () => {
  const words = { title: 'Ask for help', intro: 'Tell the team what you need.' };
  assert.deepEqual(deskHead({ name: 'IT help', description: 'Printers and laptops', icon: 'printer', color_token: 'blue' }, words),
    { title: 'IT help', subtitle: 'Printers and laptops' });
  assert.deepEqual(deskHead({ name: 'HR', description: '', icon: '', color_token: '' }, words),
    { title: 'HR', subtitle: 'Tell the team what you need.' });
  assert.deepEqual(deskHead(undefined, words), { title: 'Ask for help', subtitle: 'Tell the team what you need.' });
});

// UC-SVC-05 check 9: a desk with a logo of its own is drawn by it; one without, by its icon.
test('a desk is drawn by its own logo where it has one', () => {
  const desk = { name: 'IT help', description: '', icon: 'printer', color_token: 'blue' };
  assert.equal(deskLogoOf({ ...desk, logo_media_id: '01936f2a-7c1e-7000-8000-00000000e101' }), '01936f2a-7c1e-7000-8000-00000000e101');
  assert.equal(deskLogoOf({ ...desk, logo_media_id: null }), undefined);
  assert.equal(deskLogoOf(desk), undefined);
  assert.equal(deskLogoOf(undefined), undefined);
});

test('the desk\'s team opening its portal is told so, not shown a refusal', () => {
  assert.equal(isTeamView({ status: 403 }), true);
  assert.equal(isTeamView({ status: 404 }), false, 'a desk that is not there is a failure');
  assert.equal(isTeamView({ status: 503 }), false);
  assert.equal(isTeamView(undefined), false);
});

// UC-SVC-05 check 11 (PO-2, E-13): whoever holds READ nowhere starts in a portal - the one they
// last chose of several, else the first - and gets the portal's frame; access starts on the day.
test('an outsider starts in a portal and gets its frame; access starts on the day', () => {
  const desk = { collection_id: 'c-1', name: 'Service desk' };
  const facilities = { collection_id: 'c-2', name: 'Facilities' };
  assert.equal(portalStart(true, [desk]), '/portal/c-1');
  assert.equal(portalStart(true, [desk, facilities]), '/portal/c-1');
  assert.equal(portalStart(true, [desk, facilities], 'c-2'), '/portal/c-2', 'the portal chosen last');
  assert.equal(portalStart(true, [desk, facilities], 'gone'), '/portal/c-1');
  assert.equal(portalStart(true, []), undefined);
  assert.equal(portalStart(false, [desk]), undefined, 'somebody with access keeps their day');
  assert.equal(isOutsider(true, [desk]), true);
  assert.equal(isOutsider(true, []), false, 'nothing at all is the no-access screen, not a portal');
  assert.equal(isOutsider(false, [desk]), false, 'a member keeps the sidebar (PO-1)');
  for (const route of ['portal', 'portal-requests', 'portal-approvals', 'portal-knowledge', 'portal-article', 'portal-request', 'profile', 'item']) {
    assert.equal(isOutsiderRoute(route), true, route);
  }
  for (const route of ['home', 'search', 'hub', 'collection', 'approvals', 'portal-directory', null]) {
    assert.equal(isOutsiderRoute(route), false, String(route));
  }
});

// UC-SVC-09 check 5 (PO-3): the outsiders' tabs, approvals among them, each at its own address.
test('the portal frame has four tabs, each route under its own', () => {
  assert.equal(portalTabPath('c-1', 'start'), '/portal/c-1');
  assert.equal(portalTabPath('c-1', 'approvals'), '/portal/c-1/approvals');
  assert.equal(portalTabOf('portal-request'), 'requests');
  assert.equal(portalTabOf('portal-article'), 'knowledge');
  assert.equal(portalTabOf('portal-approvals'), 'approvals');
  assert.equal(portalTabOf('home'), undefined);
});

// UC-SVC-05 check 3 (P-12): a request's state is the server's status name, never its key; without
// a name its category's word stands; the next step follows the category.
test('a request reads by its status name and its next step, never by a key', () => {
  const words = { received: 'Received', working: 'We are on it', done: 'Done' };
  const base = { completed_at: null, created_at: '2026-11-01T09:00:00Z', updated_at: '2026-11-02T09:00:00Z' };
  assert.deepEqual(requestState({ status_name: 'In Bearbeitung', status_category: 'IN_PROGRESS', is_completed: false }, words), { label: 'In Bearbeitung', category: 'progress' });
  assert.deepEqual(requestState({ status_name: null, status_category: 'IN_PROGRESS', is_completed: false }, words), { label: 'We are on it', category: 'progress' });
  assert.deepEqual(requestState({ status_name: '  ', status_category: null, is_completed: false }, words), { label: 'Received', category: 'todo' });
  assert.deepEqual(requestState({ status_name: null, status_category: null, is_completed: true }, words), { label: 'Done', category: 'done' });
  assert.deepEqual(nextStep({ ...base, status_category: 'TODO', is_completed: false }), { kind: 'received', at: base.created_at });
  assert.deepEqual(nextStep({ ...base, status_category: 'IN_PROGRESS', is_completed: false }), { kind: 'working', at: base.updated_at });
  assert.deepEqual(nextStep({ ...base, completed_at: '2026-11-03T09:00:00Z', is_completed: true }), { kind: 'done', at: '2026-11-03T09:00:00Z' });
});

// UC-SVC-14 checks 2 and 3 (PO-1): one search over every department's request types and
// articles; my requests everywhere, newest first, each once.
test('the directory finds by every word and lists my requests everywhere newest first', () => {
  assert.equal(wordsFind('laptop bestellen', 'Laptop oder Zubehör bestellen', 'IT'), true);
  assert.equal(wordsFind('laptop drucker', 'Laptop oder Zubehör bestellen'), false);
  assert.equal(wordsFind('  ', 'anything'), false);
  const a = { id: 'a', created_at: '2026-11-01T00:00:00Z' };
  const b = { id: 'b', created_at: '2026-11-03T00:00:00Z' };
  const c = { id: 'c', created_at: '2026-11-02T00:00:00Z' };
  assert.deepEqual(newestEverywhere([[a, b], [c, b]], 2).map((each) => each.id), ['b', 'c']);
});

// UC-DOC-13 check 5 (PO-4): a shared space's pages read as a tree, parents first.
test('a space reads as an outline', () => {
  const outline = spaceOutline('root', [
    { page_id: 'p1', parent_id: 'root', title: 'Account', icon: '' },
    { page_id: 'p2', parent_id: 'p1', title: 'Lost device', icon: '' },
    { page_id: 'p3', parent_id: 'root', title: 'Orders', icon: '' },
  ]);
  assert.deepEqual(outline.map((each) => [each.page.page_id, each.depth]), [['p1', 1], ['p2', 2], ['p3', 1]]);
});

test('the portal is offered only where a collection acts as a desk', () => {
  const type = (key: string, channels?: { channel: string; closed?: boolean }[], extra: Record<string, unknown> = {}) =>
    ({ key, level: 'TASK', system: false, position: 0, archived: false, form: channels ? { channels } : undefined, ...extra }) as never;
  const incident = type('incident', [{ channel: 'PORTAL' }]);
  const story = type('story');
  assert.equal(actsAsDesk([incident, story], ['incident']), true, 'a type open to the portal, offered here, makes the desk');
  assert.equal(actsAsDesk([incident, story], undefined), false, 'a workspace-wide request type alone makes no collection a desk');
  assert.equal(actsAsDesk([incident, story], ['gone']), false, 'an offering naming no type in use falls back to all, which is no desk');
  assert.equal(actsAsDesk([story], ['story']), false, 'a Scrum collection has no portal');
  assert.equal(actsAsDesk([type('closed', [{ channel: 'PORTAL', closed: true }])], ['closed']), false, 'a closed door is no desk');
  assert.equal(actsAsDesk([type('public', [{ channel: 'PUBLIC' }])], ['public']), false, 'a public form alone is not the portal');
  assert.equal(actsAsDesk([incident, story], ['story']), false, 'the collection offers only types without the portal');
  assert.equal(actsAsDesk([type('own', [{ channel: 'PORTAL' }], { hub_id: 'h-2' })], ['own'], 'h-1'), false, 'another hub\'s type');
  assert.equal(actsAsDesk([type('arch', [{ channel: 'PORTAL' }], { archived: true })], ['arch']), false, 'an archived type');
});

test('an article never shows a requester an internal address (user-experience.md §5.6)', () => {
  const id = '01a0e2e0-0000-7000-8000-000000000901';
  const body = `# Reset your password\n\nAsk [](hubtask:entry/${id}) first.\n\n![](hubtask:media/${id})\n\n::children\n\nThen [the guide](https://example.org).`;
  assert.equal(articleText(body), '# Reset your password\n\nAsk  first.\n\nThen [the guide](https://example.org).');
});

// The team opening its own portal is refused the requester catalogue: its head and its logo are
// the collection's own, read where the team holds READ, so the logo panel has a desk to draw.
test('the team sees its own desk at its portal, though the catalogue is refused to it', () => {
  const desk = teamDeskOf({ name: 'IT help', description: null, icon: 'printer', color_token: 'blue' }, 'logo-1');
  assert.deepEqual(desk, { name: 'IT help', description: '', icon: 'printer', color_token: 'blue', logo_media_id: 'logo-1' });
  assert.equal(deskLogoOf(desk), 'logo-1');
  assert.equal(deskLogoOf(teamDeskOf({ name: 'IT help' }, null)), undefined, 'no logo draws the icon');
  assert.equal(teamDeskOf(undefined, 'logo-1'), undefined, 'nothing before the collection is read');
  assert.deepEqual(deskHead(desk, { title: 'Ask for help', intro: 'Tell the team.' }), { title: 'IT help', subtitle: 'Tell the team.' });
});

test('a new desk offers the hub\'s types open to the portal, and is a desk by them', () => {
  const type = (key: string, channels?: { channel: string; closed?: boolean }[], extra: Record<string, unknown> = {}) =>
    ({ key, level: 'TASK', system: false, position: 0, archived: false, form: channels ? { channels } : undefined, ...extra }) as never;
  const types = [
    type('incident', [{ channel: 'PORTAL' }]),
    type('story'),
    type('closed', [{ channel: 'PORTAL', closed: true }]),
    type('elsewhere', [{ channel: 'PORTAL' }], { hub_id: 'h-2' }),
    type('access', [{ channel: 'PORTAL' }], { hub_id: 'h-1' }),
  ];
  assert.deepEqual(portalTypesOf(types, 'h-1').sort(), ['access', 'incident']);
  assert.equal(actsAsDesk(types, portalTypesOf(types, 'h-1'), 'h-1'), true, 'what it offers makes it a desk');
  assert.deepEqual(portalTypesOf([type('story')], 'h-1'), [], 'a hub without a portal type has no desk to offer');
});
