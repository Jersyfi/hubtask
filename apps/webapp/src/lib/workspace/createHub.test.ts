// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The page *Create hub* as data (user-experience.md §5.2): what the gallery shows for a search and
// a family, and the one request a press sends. UC-WRK-01 check 10 (the empty hub stays offered
// whatever is searched) and UC-ID-16 check 9 (a template with *Private* is sent private).

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { EMPTY_HUB, createHubPath, familyCounts, hubRequest, landingOf, matches, presetStart, shownStarts, templateStart, type HubDraft, type Start } from './createHub.ts';

const STARTS: Start[] = [
  { value: presetStart('LIST'), family: 'simple', label: 'List', summary: 'A title and an amount, nothing else.', chips: [] },
  { value: presetStart('PROJECT'), family: 'simple', label: 'Project', summary: 'Every level, dates, people and a board.', chips: [] },
  { value: templateStart('kanban_team'), family: 'teams', label: 'Kanban team', summary: 'Cards that move across a board.', chips: [] },
  { value: templateStart('scrum_team'), family: 'teams', label: 'Scrum team', summary: 'Stories and bugs in sprints.', chips: ['Plans', 'Pages'] },
  { value: templateStart('itil_service_desk'), family: 'service', label: 'ITIL service desk', summary: 'Incidents and requests with targets.', chips: ['Service desk'] },
];
const NAMES: Record<string, string> = { simple: 'Simple', teams: 'Teams', service: 'Service' };
const familyName = (family: string) => NAMES[family] ?? family;

test('a search finds a start by its name, its summary, its products and its family, whatever the case', () => {
  const kanban = STARTS[2]!;
  assert.equal(matches(kanban, 'Teams', 'KANBAN'), true, 'by name');
  assert.equal(matches(kanban, 'Teams', 'board'), true, 'by summary');
  assert.equal(matches(STARTS[3]!, 'Teams', 'pages'), true, 'by product');
  assert.equal(matches(kanban, 'Teams', 'teams'), true, 'by family');
  assert.equal(matches(kanban, 'Teams', 'desk'), false);
  assert.equal(matches(kanban, 'Teams', '   '), true, 'an empty search finds everything');
});

test('the gallery shows each family that keeps a start, in its order, and counts what it shows', () => {
  const all = shownStarts(STARTS, 'all', '', familyName);
  assert.deepEqual(all.groups.map((group) => group.family), ['simple', 'teams', 'service']);
  assert.equal(all.count, 5);

  const teams = shownStarts(STARTS, 'teams', '', familyName);
  assert.deepEqual(teams.groups.map((group) => group.starts.map((each) => each.value)), [[templateStart('kanban_team'), templateStart('scrum_team')]]);
  assert.equal(teams.count, 2);

  const searched = shownStarts(STARTS, 'all', 'desk', familyName);
  assert.deepEqual(searched.groups.map((group) => group.family), ['service'], 'a family the search empties is left out');
  assert.equal(searched.count, 1);
});

// UC-WRK-01 check 10: *Empty hub* is no start of the gallery's families, so no search or family
// can take it away; it stands pinned before them.
test('the empty hub is never among the filtered starts, and a search without a match shows none', () => {
  const none = shownStarts(STARTS, 'all', 'nothing like this', familyName);
  assert.deepEqual(none.groups, []);
  assert.equal(none.count, 0);
  assert.ok(STARTS.every((each) => each.value !== EMPTY_HUB));
});

test('the filter counts every start of a family, whatever is searched', () => {
  assert.deepEqual(familyCounts(STARTS), [
    { family: 'simple', count: 2 },
    { family: 'teams', count: 2 },
    { family: 'service', count: 1 },
  ]);
});

const DRAFT: HubDraft = {
  start: EMPTY_HUB,
  name: '  Garden ',
  description: '',
  icon: null,
  colorToken: null,
  isPrivate: false,
  isPrivacyOffered: true,
  isPrivateOnly: false,
  answers: {},
  withSamples: true,
  acknowledged: false,
};
const KANBAN = { key: 'kanban_team', carriesSamples: true };

test('the empty hub sends the dialog\'s body: no preset, the workspace\'s default decides', () => {
  assert.deepEqual(hubRequest(DRAFT), {
    via: 'create',
    body: { type: 'HUB', parent_id: null, name: 'Garden', description: null, icon: null, color_token: null },
  });
  assert.deepEqual(hubRequest({ ...DRAFT, isPrivate: true }).body, {
    type: 'HUB', parent_id: null, name: 'Garden', description: null, icon: null, color_token: null, private: true,
  });
});

test('a preset is named by its key, a saved one by its id; the look and the description travel', () => {
  const look = { ...DRAFT, description: ' Beds ', icon: 'leaf', colorToken: 'green' };
  assert.deepEqual(hubRequest({ ...look, start: presetStart('PROJECT') }).body, {
    type: 'HUB', parent_id: null, name: 'Garden', description: 'Beds', icon: 'leaf', color_token: 'green', preset: 'PROJECT',
  });
  assert.deepEqual(hubRequest({ ...DRAFT, start: presetStart('01a0e2e0-0000-7000-8000-0000000000f1'), isPrivate: true }).body, {
    type: 'HUB', parent_id: null, name: 'Garden', description: null, icon: null, color_token: null, private: true, preset_id: '01a0e2e0-0000-7000-8000-0000000000f1',
  });
});

// UC-ID-16 check 9: a template with *Private* is made private in its one request.
test('a template goes through the template\'s operation, private where the switch is on', () => {
  const chosen = { ...DRAFT, start: templateStart('kanban_team'), template: KANBAN };
  assert.deepEqual(hubRequest(chosen), {
    via: 'template',
    body: { name: 'Garden', description: null, icon: null, color_token: null, template: 'kanban_team', samples: true },
  });
  assert.deepEqual(hubRequest({ ...chosen, isPrivate: true, withSamples: false, answers: { team: 'Ops' }, acknowledged: true }).body, {
    name: 'Garden', description: null, icon: null, color_token: null, template: 'kanban_team', private: true, answers: { team: 'Ops' }, acknowledge: true,
  });
  assert.equal((hubRequest({ ...chosen, template: { key: 'kanban_team', carriesSamples: false } }).body as { samples?: true }).samples, undefined, 'no samples where the template has none');
});

test('the switch counts only where it is drawn', () => {
  const chosen = { ...DRAFT, start: templateStart('kanban_team'), template: KANBAN, isPrivate: true, isPrivacyOffered: false };
  assert.equal(hubRequest(chosen).body.private, undefined, 'a switch the workspace does not offer sends nothing');
  assert.equal(hubRequest({ ...DRAFT, isPrivate: true, isPrivacyOffered: false }).body.private, undefined);
});

// configuration.md §6: a template asks the right to shape the workspace, so a person who may make
// only a private hub uses none.
test('a person who may make only a private hub sends a private hub without a template', () => {
  const draft = { ...DRAFT, isPrivateOnly: true, isPrivacyOffered: false, start: templateStart('kanban_team'), template: KANBAN };
  assert.deepEqual(hubRequest(draft), {
    via: 'create',
    body: { type: 'HUB', parent_id: null, name: 'Garden', description: null, icon: null, color_token: null, private: true },
  });
  assert.equal(hubRequest({ ...draft, start: presetStart('TASKS'), template: undefined }).body.private, true);
  assert.equal((hubRequest({ ...draft, start: presetStart('TASKS'), template: undefined }).body as { preset?: string }).preset, 'TASKS');
});

test('a template whose tile is no longer chosen sends the start that is', () => {
  assert.equal(hubRequest({ ...DRAFT, start: presetStart('LIST'), template: KANBAN }).via, 'create');
});

test('a hub lands where its template lands, else on the product it was made for, else on itself', () => {
  assert.equal(landingOf({ templateLanding: '/hubs/h1/desk', productScreen: '/hubs/h1/plan', hubId: 'h1' }), '/hubs/h1/desk');
  assert.equal(landingOf({ productScreen: '/hubs/h1/plan', hubId: 'h1' }), '/hubs/h1/plan');
  assert.equal(landingOf({ hubId: 'h1' }), '/hubs/h1');
});

test('an opener names the page with what it chose before', () => {
  assert.equal(createHubPath(), '/hubs/new');
  assert.equal(createHubPath({ template: 'scrum_team' }), '/hubs/new?template=scrum_team');
  assert.equal(createHubPath({ template: 'scrum_team', product: 'PLANNING' }), '/hubs/new?template=scrum_team&product=PLANNING');
  assert.equal(createHubPath({ product: 'PLANNING' }), '/hubs/new?product=PLANNING');
});
