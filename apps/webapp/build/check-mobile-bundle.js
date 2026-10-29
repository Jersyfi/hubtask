// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The Android app's bundle carries no screen of the workspace's administration or the
// installation's area (ADR-0032, UC-SYN-07 check 7): `vite build --mode mobile` puts the page that
// names the web app in the two area modules' place (`src/lib/platform/target.ts`), and this reads
// the bundle's source maps after the build and refuses one that still holds either module or any
// view they import - a screen reached some other way would ship the area by accident.

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const VIEW_IMPORT = /^\s*import\s+\w+\s+from\s+'\.\/([\w-]+\.svelte)';/gm;

/** The files a left-out area module brings: itself and every view it imports, by name. */
export function areaFiles(areas) {
  const files = new Set();
  for (const { name, source } of areas) {
    files.add(name);
    for (const match of source.matchAll(VIEW_IMPORT)) files.add(match[1]);
  }
  return files;
}

/** What of `areaFiles` the bundle's maps name: every source path whose file name is one. */
export function leftInBundle(files, sources) {
  return [...new Set(sources.map((source) => path.posix.basename(source.replaceAll('\\', '/'))))]
    .filter((name) => files.has(name))
    .sort();
}

/** What is wrong with the mobile bundle in `assets`, the source maps of a build of `app`. */
export function bundleProblems(app, assets) {
  const views = path.join(app, 'src', 'views');
  const files = areaFiles(
    ['AdministrationArea.svelte', 'InstanceArea.svelte'].map((name) => ({ name, source: fs.readFileSync(path.join(views, name), 'utf8') })),
  );
  const maps = fs.readdirSync(assets).filter((file) => file.endsWith('.js.map'));
  if (maps.length === 0) return ['no source map in the bundle - build with --mode mobile first'];
  const sources = maps.flatMap((file) => JSON.parse(fs.readFileSync(path.join(assets, file), 'utf8')).sources ?? []);
  const problems = [];
  if (!sources.some((source) => source.endsWith('InWebAppView.svelte'))) {
    problems.push('the page that names the web app is not in it - was this built with --mode mobile?');
  }
  for (const name of leftInBundle(files, sources)) {
    problems.push(`${name} is in the Android app's bundle - administration and the installation's area are the web app's`);
  }
  return problems;
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const app = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
  const problems = bundleProblems(app, path.join(app, 'dist', 'assets'));
  for (const problem of problems) console.error(`mobile bundle: ${problem}`);
  if (problems.length > 0) process.exit(1);
  console.log('mobile bundle: no screen of the two left-out areas is in it');
}
