// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The screens the owner's review of 2026-11-08 met as *This screen stopped working* in the preview
// (`docs/backlog/navigation/README.md`, R-2, R-5, R-7, R-10, R-11, R-14, R-4's settings): each opens
// on the navigation concept's workspace and draws its content, never the frame's error boundary.
// The real demo drew every one of them; the preview's stub answered them in a list's shape.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { chromium } from 'playwright';

import { ALL, APPROVALS, ENTRIES, IT, KB, MAP, MOBILE, PD, PD_BOARDS, WEB, open, served } from './kompassstub.mjs';

test.after(() => served.close());

const ON = {
  [PD.id]: ['WORKFLOWS', 'PLANNING', 'TIME', 'GOALS', 'KNOWLEDGE', 'WHITEBOARD'],
  [IT.id]: ['WORKFLOWS', 'SERVICE_DESK', 'ASSETS', 'GOALS', 'KNOWLEDGE', 'WHITEBOARD'],
};
const walk = (browser, path) => open(browser, { path, modulesOn: ALL, hubModules: ON, owner: true });
const main = (page) => page.getByRole('main');

/** Opens a screen, waits for what it must draw, and asserts no boundary and no page error. */
async function opens(t, path, drawn) {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await walk(browser, path);
  t.after(close);
  await drawn(main(page)).first().waitFor({ timeout: 10_000 });
  assert.equal(await page.locator('[data-crash]').count(), 0, `${path} drew the error boundary`);
  assert.deepEqual(failures, []);
  return page;
}

test('R-2: *Waiting for your yes* lists the approvals', async (t) => {
  await opens(t, '/approvals', (at) => at.getByText(APPROVALS[0].item_title));
});

test('R-14: *Profile -> Files* opens on its connections', async (t) => {
  await opens(t, '/profile/files', (at) => at.getByRole('heading', { level: 1 }));
});

test('R-5: a page of a hub opens, and so does an approval\'s entry', async (t) => {
  await opens(t, `/pages/${KB.id}`, (at) => at.getByText('Welcome to the team.'));
  await opens(t, `/items/${APPROVALS[0].item_id}`, (at) => at.getByText(APPROVALS[0].item_title));
});

test('R-7: a whiteboard opens on its stickies', async (t) => {
  await opens(t, `/whiteboards/${PD_BOARDS[1].id}`, (at) => at.getByText('What went well'));
  await opens(t, `/whiteboards/${MAP.id}`, (at) => at.getByText('What to change'));
});

test('R-10: a collection lists its entries, its board and its sprint show them', async (t) => {
  const first = ENTRIES.find((each) => each.collection_id === MOBILE.id);
  await opens(t, `/collections/${WEB.id}?layout=list`, (at) => at.getByText(ENTRIES.find((each) => each.collection_id === WEB.id).title));
  await opens(t, `/collections/${MOBILE.id}?layout=board`, (at) => at.getByText(first.title));
  await opens(t, `/collections/${MOBILE.id}/sprint`, (at) => at.getByText(first.title));
});

test('R-11: a collection\'s *Time* and a hub\'s budget open', async (t) => {
  await opens(t, `/collections/${MOBILE.id}/time`, (at) => at.getByRole('table'));
  await opens(t, `/hubs/${PD.id}/time/budget`, (at) => at.getByRole('navigation', { name: PD.name }));
  await opens(t, '/time?tab=report', (at) => at.getByRole('heading', { level: 1 }));
});

test('R-4: a hub\'s settings, offered to its owner, draw its settings', async (t) => {
  const page = await opens(t, `/hubs/${PD.id}/settings`, (at) => at.getByLabel('Hub name'));
  assert.equal(await main(page).getByText('Only someone who may change how this place is set up sees its settings.').count(), 0);
});

test('the administration\'s overview opens', async (t) => {
  await opens(t, '/administration', (at) => at.getByRole('heading', { level: 1 }));
});
