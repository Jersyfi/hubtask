// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readdirSync, readFileSync } from 'node:fs';

import { afterTaking, productTemplates, tryTemplateOf, workspaceTemplates } from './templates.ts';

const of = (key: string, modules: Record<string, string>, origin = 'HUBTASK', hub = true) => ({
  key,
  origin,
  document: { modules: Object.fromEntries(Object.entries(modules).map(([name, on]) => [name, { switch: on }])) },
  hub_document: hub ? {} : null,
});

// A product's first visit offers the catalogue's templates that switch its module on (user-experience.md
// §4), Hubtask's first, at most three, and only those a hub can take - one mechanism, no second list.
test('a product offers the templates that switch its module on, Hubtask\'s first, at most three', () => {
  const list = [
    of('own_desk', { service_desk: 'ON' }, 'WORKSPACE'),
    of('itil_service_desk', { workflows: 'ON', service_desk: 'ON' }),
    of('internal_it', { workflows: 'ON', service_desk: 'ON' }),
    of('kanban_team', { workflows: 'ON' }),
    of('hr_requests', { workflows: 'ON', service_desk: 'ON' }),
    of('facility_requests', { workflows: 'ON', service_desk: 'ON' }),
    of('workspace_only', { service_desk: 'ON' }, 'HUBTASK', false),
    of('desk_off', { service_desk: 'OFF' }),
  ];
  assert.deepEqual(productTemplates(list, 'SERVICE_DESK').map((each) => each.key), ['itil_service_desk', 'internal_it', 'hr_requests']);
  assert.deepEqual(productTemplates(list, 'GOALS'), []);
});

// configuration.md §6: goals and time start from templates of the workspace alone - goal sets,
// kinds of work - which the workspace takes; a template a hub takes is no such start.
test('goals and time offer the workspace templates that bring their own section', () => {
  const ws = (key: string, document: Record<string, unknown>, origin = 'HUBTASK') => ({ key, origin, document, hub_document: null });
  const list = [
    ws('own_kinds', { time_work_types: { a: {} } }, 'WORKSPACE'),
    ws('time_consulting', { time_work_types: { consulting: {} } }),
    ws('time_standard', { modules: { time: { switch: 'ON' } }, time_work_types: { development: {} } }),
    ws('goals_period', { goal_sets: { quarter: {} } }),
    ws('goals_team_health', { goal_sets: { team_health: {} } }),
    { ...ws('okr', { modules: { goals: { switch: 'ON' } } }), hub_document: {} },
    { ...ws('hub_goals', { goal_sets: { quarter: {} } }), hub_document: {} },
  ];
  assert.deepEqual(workspaceTemplates(list, 'TIME').map((each) => each.key), ['time_standard', 'time_consulting', 'own_kinds']);
  assert.deepEqual(workspaceTemplates(list, 'GOALS').map((each) => each.key), ['goals_team_health', 'goals_period'], 'the catalogue\'s order is alphabetical, the scenes\' is not');
  assert.deepEqual(workspaceTemplates(list, 'SERVICE_DESK'), []);
});

test('the workflows every template switches on is no product of its own here', () => {
  assert.deepEqual(productTemplates([of('kanban_team', { workflows: 'ON' })], 'WORKFLOWS'), []);
});

// user-experience.md §3.2: after a template is taken, the app opens the home of its main product
// - the desk for a service template, the hub's screen of it where a hub took it.
test('a taken template opens its main product\'s home', () => {
  const template = (modules: Record<string, string>) =>
    ({ key: 'k', origin: 'HUBTASK', document: { modules: Object.fromEntries(Object.entries(modules).map(([key, value]) => [key, { switch: value }])) } });
  assert.equal(afterTaking(template({ workflows: 'ON', service_desk: 'ON', knowledge: 'ON' })), '/desk');
  assert.equal(afterTaking(template({ workflows: 'ON', service_desk: 'ON', knowledge: 'ON' }), 'h-1'), '/hubs/h-1/desk');
  assert.equal(afterTaking(template({ workflows: 'ON', planning: 'ON' })), '/plans');
  assert.equal(afterTaking(template({ workflows: 'ON' })), undefined, 'no product of its own: the page stays');
  assert.equal(afterTaking(template({ service_desk: 'OFF' })), undefined);
});

// user-experience.md §3.3: *Try it* takes the template the product is mainly for - never one that
// only brings it beside another (a project's pages, a process's desk) - from the product's own
// family first; where none is mainly for it, the product's home and its switch.
test('Try it takes the template a product is mainly for', () => {
  const on = (...modules: string[]) => Object.fromEntries(modules.map((each) => [each, { switch: 'ON' }]));
  const catalogue = [
    { key: 'approvals', origin: 'HUBTASK', document: { modules: on('workflows', 'service_desk') }, hub_document: {} },
    { key: 'classic_project', origin: 'HUBTASK', document: { modules: on('workflows', 'planning', 'knowledge') }, hub_document: {} },
    { key: 'customer_support', origin: 'HUBTASK', document: { modules: on('workflows', 'service_desk') }, hub_document: {} },
    { key: 'it_asset_register', origin: 'HUBTASK', document: { modules: on('workflows') } },
  ];
  assert.equal(tryTemplateOf(catalogue, 'SERVICE_DESK')?.key, 'customer_support');
  assert.equal(tryTemplateOf(catalogue, 'PLANNING')?.key, 'classic_project');
  assert.equal(tryTemplateOf(catalogue, 'KNOWLEDGE'), undefined);
  assert.equal(tryTemplateOf(catalogue, 'ASSETS'), undefined);
  assert.equal(tryTemplateOf(catalogue, 'WORKFLOWS'), undefined);
});

// user-experience.md §3.3 and configuration.md §9: Pages and Assets each have a shipped template
// they are mainly for, so *Try it* takes the Team wiki and the Configuration items rather than the
// product's bare home.
test('Try it finds a shipped template for Pages and for Assets', () => {
  const folder = new URL('../../../../../core/application/service/config/shipped/', import.meta.url);
  const catalogue = readdirSync(folder).filter((name) => name.endsWith('.json')).sort().map((name) => {
    const shipped = JSON.parse(readFileSync(new URL(name, folder), 'utf8')) as {
      key: string; document: { modules?: Record<string, { switch?: string }> }; hub?: unknown;
    };
    return { key: shipped.key, origin: 'HUBTASK', document: shipped.document, hub_document: shipped.hub ?? null };
  });
  assert.equal(tryTemplateOf(catalogue, 'KNOWLEDGE')?.key, 'team_wiki');
  assert.equal(tryTemplateOf(catalogue, 'ASSETS')?.key, 'configuration_items');
});

// user-experience.md §4 names the ITIL service desk first among the desk's templates, and the
// `empty` scene's main way is *Desk mit ITIL-Vorlage anlegen*: it leads before a service template
// whose key sorts earlier (customer_support).
test('Try it takes the ITIL service desk for the desk', () => {
  const folder = new URL('../../../../../core/application/service/config/shipped/', import.meta.url);
  const catalogue = readdirSync(folder).filter((name) => name.endsWith('.json')).sort().map((name) => {
    const shipped = JSON.parse(readFileSync(new URL(name, folder), 'utf8')) as {
      key: string; document: { modules?: Record<string, { switch?: string }> }; hub?: unknown;
    };
    return { key: shipped.key, origin: 'HUBTASK', document: shipped.document, hub_document: shipped.hub ?? null };
  });
  assert.equal(tryTemplateOf(catalogue, 'SERVICE_DESK')?.key, 'itil_service_desk');
  assert.equal(tryTemplateOf(catalogue, 'PLANNING')?.key, 'classic_project');
});
