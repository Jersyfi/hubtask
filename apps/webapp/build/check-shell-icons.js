// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The installed apps' icons on every size (UC-WRK-33 check 5), generated from the mark's one
// app-icon source in the design system and never committed: a hand-made icon would drift from the
// mark the way the website's hand-copied favicon once did. `pnpm desktop:icons` writes them with
// the Tauri CLI; this checks, in the `shell` lane before every build, that the shell bundles
// exactly what was generated - every icon its configuration names lies in the generated directory
// and exists, the Android app's launcher icons were written into its project, and git tracks
// nothing there and ignores what is written there.

import { spawnSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

/** The directory the CLI writes the icons into, relative to the shell's directory. */
export const ICON_DIR = 'icons';

/** Where the CLI writes the Android app's launcher icons, beside the project's own resources. */
export const ANDROID_ICON_DIRS = 'gen/android/app/src/main/res/mipmap-*';
/** The largest of them, which stands for the set. */
export const ANDROID_LAUNCHER = 'gen/android/app/src/main/res/mipmap-xxxhdpi/ic_launcher.png';

/**
 * What is wrong with the shell's icons: `config` is `tauri.conf.json`, `tracked` the files git
 * tracks in the icon directory, `ignored` whether git ignores a file written there, and `exists`
 * answers for a path relative to the shell's directory.
 */
export function iconProblems({ config, tracked, ignored, exists }) {
  const problems = [];
  for (const file of tracked) problems.push(`${file} is committed - icons are generated from the mark, never by hand`);
  if (!ignored) problems.push(`${ICON_DIR}/ or ${ANDROID_ICON_DIRS} is not ignored - a generated icon could be committed`);
  if (!exists(ANDROID_LAUNCHER)) problems.push(`${ANDROID_LAUNCHER} was not generated - run pnpm desktop:icons`);
  const icons = config?.bundle?.icon;
  if (!Array.isArray(icons) || icons.length === 0) {
    problems.push('tauri.conf.json names no icon - the bundle would carry the framework\'s own');
    return problems;
  }
  for (const icon of icons) {
    if (typeof icon !== 'string' || path.posix.dirname(icon) !== ICON_DIR) {
      problems.push(`${String(icon)} is not a generated icon (${ICON_DIR}/)`);
    } else if (!exists(icon)) {
      problems.push(`${icon} was not generated - run pnpm desktop:icons`);
    }
  }
  return problems;
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const shell = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', 'src-tauri');
  const git = (...args) => spawnSync('git', args, { cwd: shell, encoding: 'utf8' });
  const listed = git('ls-files', '--', ICON_DIR, `:(glob)${ANDROID_ICON_DIRS}/**`);
  if (listed.status !== 0) {
    console.error(`shell icons: git could not list ${ICON_DIR}/: ${listed.stderr}`);
    process.exit(1);
  }
  const problems = iconProblems({
    config: JSON.parse(fs.readFileSync(path.join(shell, 'tauri.conf.json'), 'utf8')),
    tracked: listed.stdout.split('\n').filter(Boolean),
    ignored: git('check-ignore', '-q', `${ICON_DIR}/icon.png`).status === 0 && git('check-ignore', '-q', ANDROID_LAUNCHER).status === 0,
    exists: (icon) => fs.existsSync(path.join(shell, icon)),
  });
  for (const problem of problems) console.error(`shell icons: ${problem}`);
  if (problems.length > 0) process.exit(1);
  console.log('shell icons: every icon the shell bundles was generated from the mark');
}
