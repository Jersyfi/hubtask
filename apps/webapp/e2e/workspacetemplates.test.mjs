// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The templates of the workspace alone, taken from the empty states of goals and time (scenes
// emptygoals, emptytime; UC-GOL-01 check 12, UC-TIM-01 check 11; configuration.md §6): each card
// opens the template's plan, the workspace takes it - no hub is made - and the product shows what
// it brought; once one is taken, the cards are gone.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { ids, productWalk } from './productwalk.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const now = new Date();
const QUARTER = Math.floor(now.getUTCMonth() / 3) + 1;
const START = `${now.getUTCFullYear()}-${String((QUARTER - 1) * 3 + 1).padStart(2, '0')}-01`;
const END = new Date(Date.UTC(now.getUTCFullYear(), QUARTER * 3, 0)).toISOString().slice(0, 10);
const PERIOD = `${now.getUTCFullYear()} Q${QUARTER}`;

/** A goal set planned as the server plans it: the quarter around today, its name filled in. */
const goalSet = (goals) => ({ name: PERIOD, start_on: START, end_on: END, ...(goals ? { goals } : {}) });
const kind = (name, position, billable) => ({ name, position, ...(billable ? { billable: true } : {}) });

const TEMPLATES = {
  goals_customer_okr: {
    modules: { goals: { switch: 'ON' } },
    goal_sets: { customer_okr: goalSet([
      { title: 'Raise customer satisfaction', key_results: [{ title: 'Satisfaction after a closed request', kind: 'PERCENT', start: 72, target: 85 }] },
      { title: 'Keep customers longer', key_results: [{ title: 'Customers who renew', kind: 'PERCENT', start: 80, target: 90 }] },
    ]) },
  },
  goals_team_health: {
    modules: { goals: { switch: 'ON' } },
    goal_sets: { team_health: goalSet([{ title: 'Work at a sustainable pace', key_results: [] }, { title: 'Grow as a team', key_results: [] }]) },
  },
  goals_period: { modules: { goals: { switch: 'ON' } }, goal_sets: { quarter: goalSet() } },
  time_standard: {
    modules: { time: { switch: 'ON' } },
    time_work_types: { development: kind('Development', 1), meeting: kind('Meeting', 2), support: kind('Support', 3), training: kind('Training', 4), administration: kind('Administration', 5) },
  },
  time_consulting: {
    modules: { time: { switch: 'ON' } },
    time_work_types: { consulting: kind('Consulting', 1, true), implementation: kind('Implementation', 2, true), travel: kind('Travel', 3, true), internal: kind('Internal work', 4), sales: kind('Sales and offers', 5) },
  },
};

/**
 * The configuration routes a workspace template walks - the catalogue, the plan and the apply -
 * and the goals' and time's own, in memory: the apply writes what the template's sections bring.
 */
function backend() {
  const id = ids('00000c');
  const periods = [];
  const goals = [];
  const kinds = [];
  const taken = {};
  const applied = [];
  const page = (data) => ({ data, page: { next_cursor: null, has_more: false } });
  const template = (key) => ({
    key, version: 1, name: key, origin: 'HUBTASK', provenance: `${key}@1`, document: TEMPLATES[key], hub_document: null,
    collection_document: null, questions: [], created_at: '2026-11-01T00:00:00Z', update_available: false, taken_version: taken[key] ?? null,
  });
  const flags = { hides_data: false, changes_permissions: false };
  const answer = (method, path, query, body) => {
    if (path === '/config/templates' && method === 'GET') return { json: { data: Object.keys(TEMPLATES).sort().map(template) } };
    const planned = /^\/config\/templates\/([a-z_]+):plan$/.exec(path);
    if (planned && method === 'POST') {
      const key = planned[1];
      const document = TEMPLATES[key];
      const changes = [...Object.entries(document.goal_sets ?? {}), ...Object.entries(document.time_work_types ?? {})]
        .map(([block, after]) => ({ path: `${document.goal_sets ? 'goal_sets' : 'time_work_types'}.${block}`, section: document.goal_sets ? 'goal_sets' : 'time_work_types', key: block, op: 'ADD', fields: Object.keys(after), before: null, after }));
      return { json: {
        template: `${key}@1`, upgrade: [], document, mode: 'MERGE', base_hash: 'b0', plan_hash: `p-${key}`, changes, operations: [],
        refusals: [], flags, step_up: false, counts: { added: changes.length, changed: 0, removed: 0 }, hub_id: null, collection_id: null, from_version: null,
      } };
    }
    if (path === '/config:apply' && method === 'POST') {
      const key = body.template.split('@')[0];
      applied.push({ key, hub_id: body.hub_id ?? null });
      taken[key] = 1;
      for (const set of Object.values(body.document.goal_sets ?? {})) {
        let period = periods.find((each) => each.start_on === set.start_on && each.end_on === set.end_on);
        if (!period) {
          period = { id: id(), name: set.name, start_on: set.start_on, end_on: set.end_on, state: 'OPEN', version: 1 };
          periods.push(period);
        }
        for (const goal of set.goals ?? []) {
          goals.push({
            id: id(), period_id: period.id, hub_id: null, parent_id: null, title: goal.title, description: '', owner_id: null, status: 'NONE',
            confidence: null, grade: null, key_results: [], entries: { total: 0, done: 0 }, child_count: 0, progress: null, version: 1,
          });
        }
      }
      for (const [key, block] of Object.entries(body.document.time_work_types ?? {})) {
        const at = kinds.findIndex((each) => each.key === key);
        const made = { key, name: block.name, billable: block.billable === true, archived: false, position: block.position, version: 1 };
        if (at >= 0) kinds[at] = made;
        else kinds.push(made);
      }
      return { json: { version: 2, applied: true } };
    }
    if (path === '/goal-periods' && method === 'GET') return { json: page(periods) };
    if (path === '/goals' && method === 'GET') {
      const period = query.get('period_id');
      return { json: page(goals.filter((each) => !period || each.period_id === period)) };
    }
    if (path === '/portfolios' && method === 'GET') return { json: page([]) };
    if (path.startsWith('/dashboards') && method === 'GET') return { json: page([]) };
    if (path === '/time-work-types' && method === 'GET') return { json: { data: kinds } };
    const one = /^\/time-work-types\/([a-z_]+)$/.exec(path);
    if (one && method === 'PUT') {
      const made = { key: one[1], archived: false, version: 1, ...body };
      kinds.push(made);
      return { json: made };
    }
    if (path === '/time-policy' && method === 'GET') return { json: { locked_through: null, visibility: 'FULL', version: 1 } };
    if (path === '/timer' && method === 'GET') return { json: { running: false } };
    if (['/time-capacities', '/cost-rates', '/time-absences'].includes(path) && method === 'GET') return { json: { data: [] } };
    const week = /^\/timesheets\/(\d{4}-\d{2}-\d{2})$/.exec(path);
    if (week && method === 'GET') {
      const days = Array.from({ length: 7 }, (_, i) => ({ day: new Date(Date.parse(`${week[1]}T00:00:00Z`) + i * 86_400_000).toISOString().slice(0, 10), minutes: 0, locked: false }));
      return { json: { account_id: null, week_start: week[1], state: 'OPEN', submitted_minutes: 0, submitted_at: null, decided_at: null, decided_by: null, reason: '', version: 1, records: [], days, minutes: 0 } };
    }
    if (path === '/timesheets' && method === 'GET') return { json: { data: [] } };
    return undefined;
  };
  return { answer, periods, goals, kinds, applied };
}

/** Opens the card, takes the template in its dialog and waits for the dialog to close. */
async function take(page, name, titled = name) {
  const cards = page.locator('[data-empty-templates]');
  await cards.getByRole('button', { name, exact: true }).click();
  const dialog = page.getByRole('dialog', { name: titled });
  await dialog.getByRole('button', { name: 'Take template' }).click();
  await dialog.waitFor({ state: 'hidden', timeout: 5_000 });
}

for (const [name, key, shows] of [
  ['Quarterly OKR', 'goals_customer_okr', ['Raise customer satisfaction', 'Keep customers longer']],
  ['Team health', 'goals_team_health', ['Work at a sustainable pace', 'Grow as a team']],
  ['Goal period', 'goals_period', []],
]) {
  test(`chromium: an empty workspace takes ${name} from the goals' empty state, and the quarter shows it`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const world = backend();
    const walk = await productWalk(browser, { modules: ['GOALS'], answer: world.answer });
    t.after(walk.close);
    const { page } = walk;

    await page.goto(`${served.origin}/goals`);
    const cards = page.locator('[data-empty-templates]');
    await cards.waitFor({ timeout: 10_000 });
    // The three goal sets as cards beside the main way and its side way (scene emptygoals).
    assert.deepEqual(await cards.getByRole('button').evaluateAll((each) => each.map((card) => card.getAttribute('aria-label'))),
      ['Quarterly OKR', 'Team health', 'Goal period']);
    await take(page, name);
    assert.deepEqual(world.applied, [{ key, hub_id: null }], 'the workspace did not take it, or a hub did');
    await page.getByRole('combobox', { name: 'Period' }).waitFor({ timeout: 10_000 });
    assert.equal(await page.getByRole('combobox', { name: 'Period' }).locator('option:checked').innerText(), PERIOD);
    for (const title of shows) await page.getByRole('link', { name: title }).waitFor({ timeout: 5_000 });
    if (shows.length === 0) await page.getByText('No goal in this period yet.').waitFor({ timeout: 5_000 });
    assert.equal(await page.locator('[data-empty-templates]').count(), 0, 'the cards stay once a goal set is taken');
    assert.deepEqual(walk.errors(), []);
  });
}

for (const [name, key, shows] of [
  ['Kinds of work', 'time_standard', ['Development', 'Meeting', 'Support', 'Training', 'Administration']],
  ['Consulting', 'time_consulting', ['Consulting', 'Implementation', 'Travel', 'Internal work', 'Sales and offers']],
]) {
  test(`chromium: an empty workspace takes ${name} from time's empty state, and its kinds of work are there`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const world = backend();
    const walk = await productWalk(browser, { modules: ['TIME'], answer: world.answer });
    t.after(walk.close);
    const { page } = walk;

    await page.goto(`${served.origin}/time`);
    const cards = page.locator('[data-empty-templates]');
    await cards.waitFor({ timeout: 10_000 });
    assert.deepEqual(await cards.getByRole('button').evaluateAll((each) => each.map((card) => card.getAttribute('aria-label'))),
      ['Kinds of work', 'Consulting']);
    await take(page, name);
    assert.deepEqual(world.applied, [{ key, hub_id: null }], 'the workspace did not take it, or a hub did');
    await cards.waitFor({ state: 'detached', timeout: 10_000 });

    await page.goto(`${served.origin}/time/settings/kinds`);
    for (const each of shows) await page.getByRole('cell', { name: each, exact: true }).waitFor({ timeout: 10_000 });
    assert.deepEqual(walk.errors(), []);
  });
}

// A workspace with Goals or Time switched off (the demo's empty workspace) is drawn as the product's
// first empty state with its switch; whoever may switch it on is offered the same cards, and taking
// one switches the product on in the same apply (configuration.md §2, a section whose module is off).
// Time's card says its way to record, Billable; its dialog names the template it takes.
for (const [module, path, name, key, titled] of [['GOALS', '/goals', 'Team health', 'goals_team_health', 'Team health'], ['TIME', '/time', 'Billable', 'time_consulting', 'Consulting']]) {
  test(`chromium: with ${module} switched off, its first empty state offers the templates beside the switch`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const world = backend();
    const walk = await productWalk(browser, { modules: [], answer: world.answer });
    t.after(walk.close);
    const { page } = walk;

    // Drawn for the other product first and then moved here in the app: the cards follow the page.
    await page.goto(`${served.origin}${path === '/goals' ? '/time' : '/goals'}`);
    await page.locator('[data-empty-templates]').waitFor({ timeout: 10_000 });
    await page.evaluate((to) => { history.pushState({}, '', to); dispatchEvent(new PopStateEvent('popstate')); }, path);
    await page.locator('[data-module-off-note]').waitFor({ timeout: 10_000 });
    await page.locator('[data-empty-templates]').getByRole('button', { name, exact: true }).waitFor({ timeout: 10_000 });
    await take(page, name, titled);
    assert.deepEqual(world.applied, [{ key, hub_id: null }]);
    const apply = walk.written.find((each) => each.path === '/config:apply');
    assert.equal(apply.body.document.modules[module.toLowerCase()].switch, 'ON', 'the product is not switched on with it');
    assert.deepEqual(walk.errors(), []);
  });
}
