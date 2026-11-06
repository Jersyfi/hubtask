// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';

import { areaFiles, bundleProblems, leftInBundle } from './check-mobile-bundle.js';

const app = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

/** A real build of the app in `mode`, into a directory of its own. */
function build(mode) {
  const out = fs.mkdtempSync(path.join(os.tmpdir(), `hubtask-${mode}-`));
  const vite = path.join(app, 'node_modules', 'vite', 'bin', 'vite.js');
  const run = spawnSync(process.execPath, [vite, 'build', '--mode', mode, '--outDir', out, '--emptyOutDir', '--logLevel', 'error'], { cwd: app, encoding: 'utf8' });
  assert.equal(run.status, 0, run.stderr);
  return path.join(out, 'assets');
}

test('an area module brings itself and every view it imports', () => {
  const files = areaFiles([{ name: 'AdministrationArea.svelte', source: "  import PeopleView from './PeopleView.svelte';\n  import type { Resolution } from '../lib/router.ts';\n" }]);
  assert.deepEqual([...files].sort(), ['AdministrationArea.svelte', 'PeopleView.svelte']);
  assert.deepEqual(leftInBundle(files, ['../../src/views/PeopleView.svelte', '../../src/views/TodayView.svelte']), ['PeopleView.svelte']);
  assert.deepEqual(leftInBundle(files, ['..\\src\\views\\PeopleView.svelte']), ['PeopleView.svelte']);
});

test('no administration route and no route of the installation is in the Android app\'s bundle (UC-SYN-07 check 7)', () => {
  assert.deepEqual(bundleProblems(app, build('mobile')), []);
  // The check bites: the browser's bundle carries both areas.
  const browser = bundleProblems(app, build('production'));
  assert.ok(browser.some((problem) => problem.startsWith('PeopleView.svelte ')), browser.join('\n'));
  assert.ok(browser.some((problem) => problem.startsWith('InstanceOverviewView.svelte ')), browser.join('\n'));
});
