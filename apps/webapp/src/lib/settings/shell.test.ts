// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

import { ROUTES } from '../routes.ts';

// The administration's settings screens stand on the one shell (user-experience.md §5.4): its
// sections, its save bar and its leave guard. Read from the route table and the application's
// own route branches, so a screen that moves to another view is still held to it.
const ON_THE_SHELL = ['mail', 'voice', 'retention', 'backup', 'webhooks', 'workspace-settings', 'sign-in-settings', 'legal', 'ai'];

// A product's settings, drawn as its home's last tab (§8.2), stand on the same shell.
const PRODUCT_SETTINGS = ['TimeSettingsView', 'CmdbView', 'ServiceDeskView'];

const app = readFileSync(new URL('../../App.svelte', import.meta.url), 'utf8');

/** The view a route's branch in `App.svelte` loads. */
function viewOf(name: string): string | undefined {
  const branch = new RegExp(`route\\.name === '${name}'[^\\n]*\\n\\s*<LazyView load=\\{views\\.(\\w+)\\}`);
  return branch.exec(app)?.[1];
}

for (const name of ON_THE_SHELL) {
  test(`the administration's ${name} screen stands on the settings shell`, () => {
    const route = ROUTES.find((each) => each.name === name);
    assert.equal(route?.area, 'administration', `${name} is a route of the administration`);
    const view = viewOf(name);
    assert.ok(view, `App.svelte loads a view for ${name}`);
    const source = readFileSync(new URL(`../../views/${view}.svelte`, import.meta.url), 'utf8');
    assert.match(source, /<SettingsPage\b/, `${view} draws SettingsPage`);
  });
}

for (const view of PRODUCT_SETTINGS) {
  test(`the product settings ${view} stands on the settings shell`, () => {
    const source = readFileSync(new URL(`../../views/${view}.svelte`, import.meta.url), 'utf8');
    assert.match(source, /<SettingsPage\b/, `${view} draws SettingsPage`);
    assert.doesNotMatch(source, /<Tabs\b/, `${view} draws its sections on the shell, not in tabs`);
  });
}
