// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// View as file (configuration/ui.md A10): a page's values as its place's part of the document.

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { asFile, PAGE_PARTS, pagePart, pagePartFile, placePart } from './asFile.ts';
import { workspaceScreens } from './catalogue.ts';

test('a value set here is written with its value, one from above is left to the level above', () => {
  const file = asFile(
    [
      { key: 'module.planning', value: true, source: 'WORKSPACE' },
      { key: 'module.time', value: false, source: 'DEFAULT' },
      { key: 'module.agents', value: false, source: 'PLAN', lockedBy: 'PLAN' },
    ],
    'WORKSPACE',
  );
  assert.deepEqual(JSON.parse(file), {
    module: {
      planning: { set: true, value: true, locked: false },
      time: { set: false, locked: false },
      agents: { set: false, locked: false },
    },
  });
});

test('a lock held at this place is written as locked', () => {
  const file = asFile([{ key: 'sign_in.min_length', value: 12, source: 'INSTANCE', lockedBy: 'INSTANCE' }], 'INSTANCE');
  assert.deepEqual(JSON.parse(file), { sign_in: { min_length: { set: true, value: 12, locked: true } } });
});

test('values of several areas are grouped by area', () => {
  const file = asFile(
    [
      { key: 'sign_in.min_length', value: 12, source: 'WORKSPACE' },
      { key: 'legal.terms_url', value: 'https://example.org/terms', source: 'WORKSPACE' },
    ],
    'WORKSPACE',
  );
  assert.deepEqual(Object.keys(JSON.parse(file)), ['sign_in', 'legal']);
});

// A10 on every settings page: the page's own part of the configuration document.
const DOCUMENT = {
  settings: { 'sign_in.min_length': { value: 12 }, 'time.visibility': { value: 'TEAM' }, 'config.agent_apply': { value: 'ASK' } },
  rules: { triage: { name: 'Triage' } },
  collection_structure: { network: { value: 1 }, 'network.levels': { value: 3 }, 'desk.levels': { value: 2 } },
  scheme: { incident: { workflow: 'itil' } },
};

test('a page\'s file holds its sections and only its own areas of the settings', () => {
  assert.deepEqual(JSON.parse(pagePartFile(DOCUMENT, pagePart('/administration/sign-in'))), { settings: { 'sign_in.min_length': { value: 12 } } });
  assert.deepEqual(JSON.parse(pagePartFile(DOCUMENT, pagePart('/administration/rules'))), { rules: { triage: { name: 'Triage' } } });
});

test('a collection\'s file holds only its own blocks of the hub\'s collection sections', () => {
  const file = JSON.parse(pagePartFile(DOCUMENT, placePart(false, 'network')));
  assert.deepEqual(file.collection_structure, { 'network.levels': { value: 3 } });
  assert.deepEqual(JSON.parse(pagePartFile(DOCUMENT, placePart(true))).scheme, { incident: { workflow: 'itil' } });
});

// Every workspace screen the catalogue names draws *View as file* with its own part: a screen added
// to the catalogue without it fails here.
const VIEW_OF: Readonly<Record<string, string>> = {
  '/administration/sign-in': 'SignInSettingsView',
  '/administration/legal': 'SignInSettingsView',
  '/administration/workspace': 'WorkspaceSettingsView',
  '/administration/ai': 'AiSettingsView',
  '/administration/templates': 'TemplatesView',
  '/agents?tab=settings': 'AgentsView',
  '/time?tab=settings': 'TimeSettingsView',
  '/administration/rules': 'RulesView',
};
/** The screens that show the file by other means: the whole document, a module's own values. */
const OWN_FILE: Readonly<Record<string, string>> = {
  '/administration/changes': 'ChangesView',
  '/administration/modules': '../lib/workspace/ModuleSwitches',
};

test('every workspace settings screen shows its part of the configuration document', async () => {
  const { readFile } = await import('node:fs/promises');
  const source = (view: string) => readFile(new URL(`../../views/${view}.svelte`, import.meta.url), 'utf8');
  const missing: string[] = [];
  const paths = new Set([...Object.values(workspaceScreens()).map((screen) => screen.path), '/administration/rules']);
  for (const path of paths) {
    if (OWN_FILE[path]) {
      if (!/ViewAsFile|ViewSectionsAsFile|<CodeBlock/.test(await source(OWN_FILE[path]))) missing.push(path);
      continue;
    }
    const view = VIEW_OF[path];
    const part = PAGE_PARTS[path];
    if (!view || !part || (part.sections.length === 0 && (part.settings ?? []).length === 0)) {
      missing.push(path);
      continue;
    }
    if (!(await source(view)).includes(`'${path}'`) || !(await source(view)).includes('<ViewSectionsAsFile')) missing.push(path);
  }
  for (const [area, screen] of Object.entries(workspaceScreens())) {
    const part = PAGE_PARTS[screen.path];
    if (part && area !== 'structure' && !(part.settings ?? []).includes(area)) missing.push(`${area} on ${screen.path}`);
  }
  assert.deepEqual(missing, []);
});
