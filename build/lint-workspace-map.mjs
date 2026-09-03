#!/usr/bin/env node
// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Fails on any dependency edge outside the workspace map of ADR-0033.
//
// The map, in full (project-structure.md §2.1):
//
//   apps/*     → packages/*                      never another app
//   packages/* → other packages/*, acyclically   never an app
//
// The Go side has gate-architecture; until this script the workspace half of the map held by
// convention alone. It reads the manifests first, and then the imports - because a manifest
// cannot see a deep import (`@hubtask/webapp/src/...`) or a relative path that climbs out of
// one member into another, and those are exactly the edges somebody writes by accident.
//
// `--selftest` proves the checker still catches each forbidden edge kind before the real tree
// is trusted (the gate-selftest habit): a checker that cannot fail proves nothing by passing.

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const repositoryRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

/** The two kinds the map talks about, derived from the directory - not from naming convention. */
const KINDS = { apps: 'app', packages: 'package' };
// `.svelte-kit` is SvelteKit's own generated output, and its imports are SvelteKit's rather than
// ours: they reach into node_modules by relative path, which is exactly the edge this refuses.
// CI never saw it because the lint runs before anything is installed or built; a contributor
// running the documented order locally did.
const SKIP_DIRS = new Set(['node_modules', 'dist', '.vite', '.turbo', '.svelte-kit', 'src-tauri', 'fonts']);
const SOURCE_EXTENSIONS = new Set(['.ts', '.tsx', '.js', '.jsx', '.svelte', '.css']);

/** Read the members: every directory under apps/ and packages/ that carries a package.json. */
export function readMembers(root = repositoryRoot) {
  const members = [];
  for (const [dir, kind] of Object.entries(KINDS)) {
    const absolute = path.join(root, dir);
    if (!fs.existsSync(absolute)) continue;
    for (const entry of fs.readdirSync(absolute, { withFileTypes: true })) {
      if (!entry.isDirectory()) continue;
      const manifestPath = path.join(absolute, entry.name, 'package.json');
      if (!fs.existsSync(manifestPath)) continue;
      const manifest = JSON.parse(fs.readFileSync(manifestPath, 'utf8'));
      members.push({
        name: manifest.name,
        kind,
        dir: path.join(dir, entry.name),
        dependencies: [
          ...Object.keys(manifest.dependencies ?? {}),
          ...Object.keys(manifest.devDependencies ?? {}),
          ...Object.keys(manifest.peerDependencies ?? {}),
        ],
      });
    }
  }
  return members;
}

/**
 * The manifest half: every declared edge between members must be on the map, and the
 * package-to-package edges must not close a cycle.
 */
export function manifestViolations(members) {
  const byName = new Map(members.map((m) => [m.name, m]));
  const problems = [];
  const packageEdges = new Map();

  for (const member of members) {
    for (const dependency of member.dependencies) {
      const target = byName.get(dependency);
      if (!target) continue; // a real third-party dependency; the map has nothing to say
      if (target.kind === 'app') {
        problems.push(
          `${member.dir}: depends on ${target.dir} - nothing may depend on an app ` +
            (member.kind === 'app' ? '(apps share packages, never each other)' : '(a package that knows an app is that app)'),
        );
      } else if (member.kind === 'package') {
        packageEdges.set(member.name, [...(packageEdges.get(member.name) ?? []), target.name]);
      }
    }
  }

  // Cycles among packages. Depth-first with a path stack: small graph, exact answer.
  const visiting = new Set();
  const done = new Set();
  const walk = (name, trail) => {
    if (done.has(name)) return;
    if (visiting.has(name)) {
      problems.push(`packages: dependency cycle ${[...trail, name].join(' → ')}`);
      return;
    }
    visiting.add(name);
    for (const next of packageEdges.get(name) ?? []) walk(next, [...trail, name]);
    visiting.delete(name);
    done.add(name);
  };
  for (const name of packageEdges.keys()) walk(name, []);

  return problems;
}

/**
 * The one file a member may reach out of itself for: the message catalogue.
 *
 * `locales/en.json` is the product's single source of display text (i18n-l10n.md §3) and it lives
 * at the repository root because the Go binary embeds it - `locales/Embed.go` exists for that
 * reason alone. The client renders the same codes from the same file, and the alternative to this
 * import is a copy under `apps/`, which is the one thing a source of truth must not have.
 *
 * It is deliberately the file rather than the directory: an escape from a member towards anything
 * else, including anything else in `locales/`, stays a violation. The map this checker enforces is
 * about edges *between members* (project-structure.md §2.1), and the catalogue is not a member -
 * it is data both halves of the product read.
 */
const SHARED_CATALOGUE = path.join('locales', 'en.json');

/**
 * The import half: the edges a manifest cannot see. A relative import that resolves outside its
 * own member is always wrong - a cross-member path couples to another member's file layout, and
 * the sanctioned route is the package name. A member imported by name must obey the same map as
 * a declared dependency (this also catches an edge that was never declared).
 */
export function importViolations(member, members, relativeFile, source) {
  const byName = new Map(members.map((m) => [m.name, m]));
  const problems = [];
  for (const match of source.matchAll(/(?:from|import)\s*\(?\s*['"]([^'"]+)['"]/g)) {
    const specifier = match[1];
    if (specifier.startsWith('.')) {
      const resolved = path.normalize(path.join(path.dirname(relativeFile), specifier));
      if (
        !resolved.startsWith(member.dir + path.sep) &&
        resolved !== member.dir &&
        resolved !== SHARED_CATALOGUE
      ) {
        problems.push(`${relativeFile}: relative import '${specifier}' leaves ${member.dir}`);
      }
      continue;
    }
    const packageName = specifier.startsWith('@')
      ? specifier.split('/').slice(0, 2).join('/')
      : specifier.split('/')[0];
    const target = byName.get(packageName);
    if (!target || target.name === member.name) continue;
    if (target.kind === 'app') {
      problems.push(`${relativeFile}: imports ${target.dir} - nothing may depend on an app`);
    } else if (!member.dependencies.includes(target.name)) {
      problems.push(`${relativeFile}: imports ${target.name} without declaring it in package.json`);
    }
  }
  return problems;
}

function* sourceFiles(dir) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const joined = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (!SKIP_DIRS.has(entry.name)) yield* sourceFiles(joined);
    } else if (SOURCE_EXTENSIONS.has(path.extname(entry.name))) {
      yield joined;
    }
  }
}

function checkTree(root = repositoryRoot) {
  const members = readMembers(root);
  const problems = [...manifestViolations(members)];
  for (const member of members) {
    for (const file of sourceFiles(path.join(root, member.dir))) {
      const relative = path.relative(root, file);
      problems.push(...importViolations(member, members, relative, fs.readFileSync(file, 'utf8')));
    }
  }
  return { members, problems };
}

// ---------------------------------------------------------------------------------------------
// The selftest: in-memory members and sources carrying one violation of each forbidden kind.
// ---------------------------------------------------------------------------------------------

function selftest() {
  const fixture = (deps) => [
    { name: '@x/webapp', kind: 'app', dir: 'apps/webapp', dependencies: deps.webapp ?? [] },
    { name: '@x/website', kind: 'app', dir: 'apps/website', dependencies: deps.website ?? [] },
    { name: '@x/a', kind: 'package', dir: 'packages/a', dependencies: deps.a ?? [] },
    { name: '@x/b', kind: 'package', dir: 'packages/b', dependencies: deps.b ?? [] },
  ];
  const cases = [
    ['an app depending on an app', () => manifestViolations(fixture({ webapp: ['@x/website'] }))],
    ['a package depending on an app', () => manifestViolations(fixture({ a: ['@x/webapp'] }))],
    ['a cycle between packages', () => manifestViolations(fixture({ a: ['@x/b'], b: ['@x/a'] }))],
    [
      'an import of an app',
      () =>
        importViolations(fixture({})[2], fixture({}), 'packages/a/src/x.ts', "import y from '@x/webapp'"),
    ],
    [
      'an undeclared member import',
      () => importViolations(fixture({})[0], fixture({}), 'apps/webapp/src/x.ts', "import y from '@x/a'"),
    ],
    [
      'a relative import leaving the member',
      () =>
        importViolations(fixture({})[0], fixture({}), 'apps/webapp/src/x.ts', "import y from '../../packages/a/src/y.ts'"),
    ],
    [
      'an escape to the repository root that is not the catalogue',
      () =>
        importViolations(fixture({})[0], fixture({}), 'apps/webapp/src/x.ts', "import y from '../../locales/Embed.go'"),
    ],
  ];

  // …and the exception itself, which is only worth having if it is exactly one file wide.
  const catalogue = importViolations(
    fixture({})[0],
    fixture({}),
    'apps/webapp/src/lib/i18n/catalogue.ts',
    "import english from '../../../../../locales/en.json' with { type: 'json' }",
  );
  if (catalogue.length > 0) {
    console.error(`workspace map selftest: the shared catalogue was flagged: ${catalogue[0]}`);
    return false;
  }
  const clean = manifestViolations(fixture({ webapp: ['@x/a'], a: ['@x/b'] }));
  if (clean.length > 0) {
    console.error(`workspace map selftest: a legal map was flagged: ${clean[0]}`);
    return false;
  }
  for (const [what, run] of cases) {
    if (run().length === 0) {
      console.error(`workspace map selftest: ${what} was not caught`);
      return false;
    }
  }
  console.log(`workspace map selftest: all ${cases.length} planted violations caught`);
  return true;
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  if (process.argv.includes('--selftest')) {
    process.exit(selftest() ? 0 : 1);
  }
  const { members, problems } = checkTree();
  if (problems.length > 0) {
    for (const problem of problems) console.error(`workspace map: ${problem}`);
    console.error(`\n${problems.length} edge(s) outside the map of ADR-0033 (project-structure.md §2.1).`);
    process.exit(1);
  }
  console.log(`workspace map: ${members.length} members, every edge on the map of ADR-0033`);
}
