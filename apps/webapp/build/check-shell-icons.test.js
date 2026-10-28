// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import fs from 'node:fs';
import { test } from 'node:test';

import { iconProblems } from './check-shell-icons.js';

const config = JSON.parse(fs.readFileSync(new URL('../src-tauri/tauri.conf.json', import.meta.url), 'utf8'));
const scripts = JSON.parse(fs.readFileSync(new URL('../package.json', import.meta.url), 'utf8')).scripts;
const clean = { config, tracked: [], ignored: true, exists: () => true };

test('the shell bundles generated icons only, and they come from the mark (UC-WRK-33 check 5)', () => {
  assert.deepEqual(iconProblems(clean), []);
  assert.match(
    scripts['desktop:icons'],
    /^tauri icon \.\.\/\.\.\/packages\/design-system\/dist\/mark\/app-icon\.svg --output src-tauri\/icons && node build\/check-shell-icons\.js$/,
    'the icons are written from the design system\'s app-icon source, then checked',
  );
});

test('a committed icon, a missing one or one from elsewhere is refused', () => {
  const cases = [
    [{ ...clean, tracked: ['icons/icon.png'] }, /committed/],
    [{ ...clean, ignored: false }, /not ignored/],
    [{ ...clean, exists: (icon) => icon !== 'icons/icon.ico' }, /icons\/icon\.ico was not generated/],
    [{ ...clean, config: { bundle: { icon: ['art/icon.png'] } } }, /not a generated icon/],
    [{ ...clean, config: { bundle: {} } }, /names no icon/],
  ];
  for (const [input, want] of cases) {
    const problems = iconProblems(input);
    assert.equal(problems.length, 1, JSON.stringify(problems));
    assert.match(problems[0], want);
  }
});
