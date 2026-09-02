// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// A component without a story is a build failure.
//
// This is the design system's counterpart to the parity gate on the Go side (ADR-0037). The Go
// version refuses a use case that is reachable through REST but not through MCP; this one refuses
// a component that exists in the tree but nowhere a person can look at it, and a component that
// exists in the tree but in none of `design-system.md` §4's waves.
//
// The last rule is the one that matters in a year. §4 is an inventory, and an inventory drifts
// from the tree the first time somebody adds a component without saying so. Neither list is
// allowed to move alone.
//
// Plain Node, no dependencies, no TypeScript parser: the story format is a literal object by
// design, and a gate that needed a compiler would be a gate nobody runs before committing. Where
// the reading is fragile it fails loudly rather than quietly finding nothing - see FLOOR below.

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const packageRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const repositoryRoot = path.resolve(packageRoot, '..', '..');
const SPECIFICATION = path.join(repositoryRoot, 'docs', 'design', 'design-system.md');

/** Where a story may live. Components in src/, and the workbench's own fixtures. */
const STORY_ROOTS = [path.join('src'), path.join('workbench', 'fixtures')];

/**
 * The smallest number of components each wave must still yield. §4 is prose and a table, and a
 * parser over prose fails by finding nothing rather than by throwing. These floors turn that
 * silence into a failure: an edit that breaks the reading is reported as a broken reading, not as
 * a clean run.
 */
const FLOOR = { 'Wave 0': 4, 'Wave 1': 15, 'Wave 2': 10, 'Wave 3': 15, 'Wave 4': 6 };

const read = (file) => fs.readFileSync(file, 'utf8');

function* walk(dir) {
  if (!fs.existsSync(dir)) return;
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const joined = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (entry.name !== 'node_modules' && entry.name !== 'dist') yield* walk(joined);
    } else {
      yield joined;
    }
  }
}

// ---------------------------------------------------------------------------------------------
// The vocabularies, read from the one file that declares each of them.
// ---------------------------------------------------------------------------------------------

/** The axis ids, from workbench/lib/axes.ts - never a second copy of the list. */
export function readAxisIds(source = read(path.join(packageRoot, 'workbench', 'lib', 'axes.ts'))) {
  const match = source.match(/export const AXIS_IDS = \[([^\]]+)\]/);
  if (!match) throw new Error('workbench/lib/axes.ts: AXIS_IDS is no longer readable');
  return [...match[1].matchAll(/'([a-z]+)'/g)].map((m) => m[1]);
}

/** The story statuses, from workbench/lib/story.ts, for the same reason. */
export function readStatuses(source = read(path.join(packageRoot, 'workbench', 'lib', 'story.ts'))) {
  const match = source.match(/export type Status =([^;]+);/);
  if (!match) throw new Error('workbench/lib/story.ts: Status is no longer readable');
  return [...match[1].matchAll(/'([a-z]+)'/g)].map((m) => m[1]);
}

/**
 * §4's inventory: the component names the specification plans, by wave. Waves 1, 2 and 4 are
 * prose separated by `·`; wave 3 is a table whose first column names the component in backticks,
 * sometimes two of them joined by `+`.
 */
export function readInventory(markdown = read(SPECIFICATION)) {
  const section = markdown.slice(markdown.indexOf('## 4. Component inventory'));
  const body = section.slice(0, section.indexOf('\n## ', 1));
  const waves = new Map();

  for (const match of body.matchAll(/### (Wave \d)[^\n]*\n([\s\S]*?)(?=\n### |$)/g)) {
    const [, wave] = match;
    // The horizontal rule before the next heading is not part of the wave.
    const text = match[2].replace(/\n-{3,}\s*$/, '');
    const names = new Set();

    // A wave-3 row names one or two components in its first cell (`LabelChip` + `LabelPicker`),
    // so every backticked identifier in that cell counts - not only the first.
    for (const row of text.matchAll(/^\|([^|]*)\|/gm)) {
      for (const backticked of row[1].matchAll(/`([^`]+)`/g)) {
        const trimmed = backticked[1].trim();
        if (/^[A-Z][A-Za-z]*$/.test(trimmed)) names.add(trimmed);
      }
    }

    if (names.size === 0) {
      // Only the first paragraph after the heading. A wave lists its names there and may then
      // explain itself in prose; reading the whole block glues the last name to the first
      // sentence of that explanation and loses it - silently, which is the failure mode this
      // whole file is built to avoid.
      const prose = text
        .trim()
        .split(/\n\s*\n/)[0]
        .replace(/\*\([^)]*\)\*/g, '') // the italic asides after a name
        .replace(/\n/g, ' ');
      for (const piece of prose.split('·')) {
        const trimmed = piece.replace(/[`*]/g, '').trim();
        if (/^[A-Z][A-Za-z]*$/.test(trimmed)) names.add(trimmed);
      }
    }

    waves.set(wave, [...names]);
  }

  return waves;
}

export function inventoryProblems(waves, floor = FLOOR) {
  const problems = [];
  for (const [wave, minimum] of Object.entries(floor)) {
    const found = waves.get(wave);
    if (!found) {
      problems.push(`design-system.md §4: ${wave} is gone, or its heading changed`);
    } else if (found.length < minimum) {
      problems.push(
        `design-system.md §4: only ${found.length} component name(s) read from ${wave}, expected at least ` +
          `${minimum} - the section changed shape and this checker no longer understands it`,
      );
    }
  }
  return problems;
}

// ---------------------------------------------------------------------------------------------
// The story modules, read as text. The format is a literal object; that is what makes this legal.
// ---------------------------------------------------------------------------------------------

/** Every field the workbench needs, plus the named exports it renders. */
export function readStoryModule(source) {
  const title = source.match(/title:\s*'([^']*)'/);
  const component = source.match(/component:\s*([A-Za-z_$][\w$]*)/);
  const status = source.match(/status:\s*'([^']*)'/);
  const axes = source.match(/axes:\s*\[([^\]]*)\]/);
  return {
    title: title?.[1],
    component: component?.[1],
    status: status?.[1],
    axes: axes ? [...axes[1].matchAll(/'([a-z]+)'/g)].map((m) => m[1]) : undefined,
    stories: [...source.matchAll(/^export const (\w+)/gm)].map((m) => m[1]),
  };
}

export function storyProblems(relative, source, axisIds, statuses) {
  const problems = [];
  const module = readStoryModule(source);
  const at = (message) => `${relative}: ${message}`;

  if (!module.title) problems.push(at('no `title` in the default export'));
  if (!module.component) problems.push(at('no `component` in the default export'));
  if (!module.status) problems.push(at('no `status` in the default export'));
  else if (!statuses.includes(module.status)) {
    problems.push(at(`status '${module.status}' is not one of ${statuses.join(', ')}`));
  }
  if (!module.axes) problems.push(at('no `axes` in the default export'));
  else if (module.axes.length === 0) {
    problems.push(at('`axes` is empty - name the axes that carry a rule for this component'));
  } else {
    for (const axis of module.axes) {
      if (!axisIds.includes(axis)) problems.push(at(`unknown axis '${axis}'`));
    }
  }
  if (module.stories.length === 0) {
    problems.push(at('a story module with no story exports - nothing here can be rendered'));
  }

  return problems;
}

// ---------------------------------------------------------------------------------------------
// The tree.
// ---------------------------------------------------------------------------------------------

export function checkTree(root = packageRoot, specification = SPECIFICATION) {
  const axisIds = readAxisIds();
  const statuses = readStatuses();
  const waves = readInventory(read(specification));
  const problems = inventoryProblems(waves);
  const planned = new Set([...waves.values()].flat());

  const components = [];
  const stories = [];
  for (const storyRoot of STORY_ROOTS) {
    for (const file of walk(path.join(root, storyRoot))) {
      const relative = path.relative(root, file);
      if (file.endsWith('.stories.ts')) stories.push({ file, relative });
      else if (file.endsWith('.svelte')) components.push({ file, relative, storyRoot });
    }
  }

  for (const component of components) {
    const name = path.basename(component.file, '.svelte');
    // A leading underscore is the way to say "this is a part, not a component". Without it every
    // internal fragment would need a story of its own.
    if (name.startsWith('_')) continue;

    const expected = component.file.replace(/\.svelte$/, '.stories.ts');
    if (!fs.existsSync(expected)) {
      problems.push(
        `${component.relative}: no ${path.basename(expected)} beside it - a component nobody can ` +
          'see in every state is not finished',
      );
    }

    // The inventory rule applies to components, not to the workbench's own fixtures: a fixture is
    // a tool for checking the tool and §4 does not plan it.
    if (component.storyRoot === 'src' && !planned.has(name)) {
      problems.push(
        `${component.relative}: '${name}' is in no wave of design-system.md §4 - add it to the ` +
          'inventory, or the specification and the tree have become two different lists',
      );
    }
  }

  for (const story of stories) {
    problems.push(...storyProblems(story.relative, read(story.file), axisIds, statuses));
  }

  return { problems, components: components.length, stories: stories.length, planned: planned.size };
}

// ---------------------------------------------------------------------------------------------
// The selftest: a checker that cannot fail proves nothing by passing.
// ---------------------------------------------------------------------------------------------

const AXES = ['theme', 'dir', 'text'];
const STATUSES = ['fixture', 'draft', 'stable'];

const GOOD_STORY = `
export default {
  title: 'Wave 1/Button',
  component: Button,
  status: 'draft',
  axes: ['theme', 'dir'],
} satisfies StoryMeta;
export const resting: Story = { name: 'Resting' };
`;

/** A wave that lists its names and then explains itself - the shape that lost a name once. */
const WAVE_WITH_PROSE = `## 4. Component inventory

### Wave 0 — the primitives (4)
\`Box\` · \`Stack\` · \`Inline\` · \`VisuallyHidden\`

An explanation follows the names, and it must not be read as one of them.

## 5. Naming
`;

export function selftest() {
  const cases = [
    ['a missing title', () => storyProblems('x', GOOD_STORY.replace(/title:[^\n]*\n/, ''), AXES, STATUSES)],
    ['a missing component', () => storyProblems('x', GOOD_STORY.replace(/component:[^\n]*\n/, ''), AXES, STATUSES)],
    ['a missing status', () => storyProblems('x', GOOD_STORY.replace(/status:[^\n]*\n/, ''), AXES, STATUSES)],
    ['an unknown status', () => storyProblems('x', GOOD_STORY.replace("'draft'", "'settled'"), AXES, STATUSES)],
    ['a missing axes list', () => storyProblems('x', GOOD_STORY.replace(/axes:[^\n]*\n/, ''), AXES, STATUSES)],
    ['an empty axes list', () => storyProblems('x', GOOD_STORY.replace("['theme', 'dir']", '[]'), AXES, STATUSES)],
    ['an unknown axis', () => storyProblems('x', GOOD_STORY.replace("'dir'", "'density'"), AXES, STATUSES)],
    [
      'a module with no story exports',
      () => storyProblems('x', GOOD_STORY.replace(/export const resting[^\n]*\n/, ''), AXES, STATUSES),
    ],
    ['a wave that stopped parsing', () => inventoryProblems(new Map([['Wave 1', ['Button']]]), { 'Wave 1': 15 })],
    ['a wave that disappeared', () => inventoryProblems(new Map(), { 'Wave 1': 15 })],
  ];

  const clean = storyProblems('x', GOOD_STORY, AXES, STATUSES);
  if (clean.length > 0) {
    console.error(`check-stories selftest: a correct story module was flagged: ${clean[0]}`);
    return false;
  }

  // Not a planted violation but a planted *reading*: the last name before a paragraph of prose
  // was silently swallowed once, and a floor cannot catch the loss of one name out of four.
  const read = readInventory(WAVE_WITH_PROSE).get('Wave 0') ?? [];
  const expected = ['Box', 'Stack', 'Inline', 'VisuallyHidden'];
  if (read.join(',') !== expected.join(',')) {
    console.error(`check-stories selftest: a wave with prose read as [${read}], expected [${expected}]`);
    return false;
  }
  for (const [what, run] of cases) {
    if (run().length === 0) {
      console.error(`check-stories selftest: ${what} was not caught`);
      return false;
    }
  }
  console.log(`check-stories selftest: all ${cases.length} planted violations caught`);
  return true;
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  if (process.argv.includes('--selftest')) {
    process.exit(selftest() ? 0 : 1);
  }
  const { problems, components, stories, planned } = checkTree();
  if (problems.length > 0) {
    for (const problem of problems) console.error(problem);
    console.error(`\n${problems.length} problem(s). See ADR-0037 and design-system.md §4.`);
    process.exit(1);
  }
  console.log(
    `check-stories: ${components} component(s), ${stories} story module(s), ` +
      `${planned} planned in design-system.md §4`,
  );
}
