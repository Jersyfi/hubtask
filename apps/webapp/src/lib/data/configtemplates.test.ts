// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The *Templates* gallery: four origins in order, what each template offers, and an upgrade that
// never overwrites what the workspace changed itself without the person choosing it.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { SECTIONS, answersOf, byOrigin, changeWords, galleryScope, modulesToSwitchOn, offerOf, offersSamples, refusalWords, takenFirst } from './configtemplates.ts';

test('the gallery draws the four origins in order and leaves an empty one out', () => {
  const groups = byOrigin([
    { key: 'agency', origin: 'WORKSPACE' as const },
    { key: 'list', origin: 'HUBTASK' as const },
    { key: 'mine', origin: 'OWN' as const },
  ]);
  assert.deepEqual(groups.map((group) => group.origin), ['HUBTASK', 'WORKSPACE', 'OWN']);
});

test('a template not taken is offered, a taken one only when a newer version exists', () => {
  assert.equal(offerOf({ origin: 'HUBTASK', taken_version: null, update_available: false }), 'TAKE');
  assert.equal(offerOf({ origin: 'WORKSPACE', taken_version: 2, update_available: true }), 'UPDATE');
  assert.equal(offerOf({ origin: 'WORKSPACE', taken_version: 3, update_available: false }), 'IN_USE');
});

test('an upgrade takes what only the template changed and leaves every conflict', () => {
  assert.deepEqual(
    takenFirst([
      { path: 'types.incident', state: 'TAKE' },
      { path: 'roles.lead', state: 'CONFLICT' },
      { path: 'structure.views', state: 'TAKE' },
    ]),
    ['types.incident', 'structure.views'],
  );
});

test('a plan is asked with each answer, a default where none was given, and nothing for an open question', () => {
  const questions = [{ key: 'first_group' }, { key: 'greeting', default: 'Hello' }, { key: 'tone', default: null }];
  assert.deepEqual(answersOf(questions, { first_group: 'g-1', tone: '  ' }), { first_group: 'g-1', greeting: 'Hello' });
  assert.deepEqual(answersOf(undefined, { first_group: 'g-1' }), {});
});

test('sample entries are offered to a hub taking a template that brings some, never on an upgrade', () => {
  const desk = { origin: 'HUBTASK' as const, taken_version: null, update_available: false, hub_document: { samples: { vpn: { title: 'VPN' } } } };
  assert.equal(offersSamples(desk, 'hub'), true);
  assert.equal(offersSamples(desk, undefined), false, 'the workspace takes no samples');
  assert.equal(offersSamples({ ...desk, taken_version: 7, update_available: true }, 'hub'), false, 'an upgrade offers none');
  assert.equal(offersSamples({ ...desk, hub_document: { samples: {} } }, 'hub'), false);
  assert.equal(offersSamples({ ...desk, hub_document: null }, 'hub'), false);
});

test("a collection's settings draw the collection's gallery, not its hub's", () => {
  assert.equal(galleryScope(undefined, 'collection'), 'collection');
  assert.equal(galleryScope('hub', 'collection'), 'collection');
  assert.equal(galleryScope('hub', undefined), 'hub');
  assert.equal(galleryScope(undefined, undefined), 'workspace');
});

test('a template refused only for modules that are off names them, and nothing else is one switch away', () => {
  const off = (module: string, path: string) => ({ path, code: 'module.off', params: { module } });
  assert.deepEqual(modulesToSwitchOn([off('WORKFLOWS', '/types'), off('WORKFLOWS', '/workflows'), off('SERVICE_DESK', '/sla_policies')]), ['WORKFLOWS', 'SERVICE_DESK']);
  assert.deepEqual(modulesToSwitchOn([off('WORKFLOWS', '/workflows'), { path: '/roles', code: 'config.section_withheld' }]), [], 'another refusal stays');
  assert.deepEqual(modulesToSwitchOn([]), []);
});

test('a plan is said in words: no message code and no key path is shown', () => {
  assert.deepEqual(changeWords({ section: 'structure', key: 'automation', op: 'ADD' }),
    { code: 'app.templates.change_add', section: 'app.templates.section_structure', key: 'automation' });
  assert.equal(changeWords({ section: 'something_new', key: 'k', op: 'REMOVE' }).section, 'app.templates.section_other');
  // §5.4: a block with a name is named by it, a module by its product's word - never a key.
  assert.equal(changeWords({ section: 'roles', key: 'cab_member', op: 'ADD', after: { name: 'CAB member' } }).key, 'CAB member');
  assert.equal(changeWords({ section: 'fields', key: 'responding_group', op: 'REMOVE', before: { name: 'Responding group' }, after: null }).key, 'Responding group');
  assert.deepEqual(changeWords({ section: 'modules', key: 'knowledge', op: 'CHANGE', after: { switch: 'ON' } }),
    { code: 'app.templates.change_module_on', section: 'app.templates.section_modules', key: 'knowledge', module: 'app.modules.name_knowledge' });
  assert.equal(changeWords({ section: 'modules', key: 'goals', op: 'ADD', after: { switch: 'OFF' } }).code, 'app.templates.change_module_off');
  assert.equal(changeWords({ section: 'modules', key: 'goals', op: 'REMOVE', before: { switch: 'ON' }, after: null }).code, 'app.templates.change_remove',
    'a module left to the level above is no switch either way');
  assert.deepEqual(refusalWords({ path: '/workflows', code: 'module.off', params: { module: 'WORKFLOWS' } }),
    { section: 'app.templates.section_workflows', code: 'app.templates.needs_module', module: 'app.modules.name_workflows' });
  assert.deepEqual(refusalWords({ path: '/settings/ai.source', code: 'config.section_withheld' }),
    { section: 'app.templates.section_settings', code: 'config.section_withheld' });
});

test('every section a template may bring has its word in the catalogue', async () => {
  const { readFileSync } = await import('node:fs');
  const catalogue = JSON.parse(readFileSync(new URL('../../../../../locales/en.json', import.meta.url), 'utf8'));
  for (const section of [...SECTIONS, 'other']) assert.ok(catalogue[`app.templates.section_${section}`], `no word for ${section}`);
});

test('a plan is said in words, refusals first, without its path or its JSON', async () => {
  const { planLines } = await import('./configtemplates.ts');
  const said = (code: string, params: Readonly<Record<string, string | number>> = {}) =>
    `${code}(${Object.entries(params).map(([name, value]) => `${name}=${value}`).join(',')})`;
  const lines = planLines(
    {
      refusals: [{ path: '/roles/auditor', code: 'config.role_in_use', params: { holders: '2' } }],
      changes: [
        { section: 'types', key: 'incident', op: 'ADD', after: { name: 'Incident' } },
        { section: 'modules', key: 'PAGES', op: 'CHANGE', after: { switch: 'ON' } },
        { section: 'roles', key: 'auditor', op: 'REMOVE', before: { name: 'Auditor' } },
      ],
    },
    said,
  );
  assert.deepEqual(lines.map((line) => line.tone), ['refuse', 'add', 'change', 'remove']);
  assert.match(lines[1]!.text, /Incident/);
  assert.match(lines[2]!.text, /app\.modules\.name_pages/);
  for (const line of lines) assert.doesNotMatch(line.text, /\/roles\/|[{}"]/);
});
