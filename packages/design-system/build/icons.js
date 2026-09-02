// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Generates src/icons/base.ts from the declared subset of Lucide (ADR-0041).
//
// The list below is the whole mechanism. Lucide ships 1792 icons; this file names the ones the
// product uses, and nothing else is written into the repository - so there is no thousand-glyph
// bundle for a tree-shaker to prune, and the cost of the icon set is the cost of the list. An
// icon nobody declared is not in `src/`, which is a stronger guarantee than an import a bundler
// might or might not drop (ADR-0028: the bundle is carried into the binary byte for byte).
//
// The output is committed, for the reason `LabelTokens.go` is: a checkout that never ran
// `pnpm install` still builds, and drift shows up as a diff rather than as a missing glyph.
//
// Run with `make icons`.

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const packageRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

/**
 * The declared subset, grouped by what asks for it. A group is a claim: the components of that
 * wave need these and no others. Adding a name here is the only way an icon enters the product.
 */
export const DECLARED = {
  'Actions — wave 1': [
    'check',
    'plus',
    'minus', // the third state of a checkbox: some, not all
    'x',
    'pencil',
    'trash-2',
    'archive',
    'copy',
    'link',
    'search',
    'settings',
    'log-out',
    'ellipsis',
    'grip-vertical',
    'funnel', // a filter; lucide renamed `filter` to `funnel`
  ],
  'Navigation — wave 2': [
    'chevron-up',
    'chevron-down',
    'chevron-left',
    'chevron-right',
    'chevrons-up-down',
    'arrow-left',
    'arrow-right',
    'external-link',
    'menu',
    'panel-left',
  ],
  'State and feedback': [
    'circle-check',
    'circle-alert',
    'triangle-alert',
    'info',
    'loader-circle',
    'ban',
    'eye',
    'eye-off',
    'sun',
    'moon',
  ],
  'Domain nouns Lucide already says well': [
    'calendar', // a due date
    'clock', // a time, and the history
    'bell', // a reminder
    'user', // an assignee
    'users', // members
    'user-check', // an assignment
    'paperclip', // an attachment
    'message-square', // a comment
    'tag', // a label
    'image', // a cover
    'file-text', // a note
    'repeat', // a recurrence rule
  ],
};

const SOURCE = path.join(packageRoot, 'node_modules', 'lucide-static', 'icon-nodes.json');
const DESTINATION = path.join(packageRoot, 'src', 'icons', 'base.ts');

/**
 * Only `class`. It is tempting to strip `width` and `height` here too, on the grounds that the
 * wrapper sets them - and it costs the set every `rect`, because on a child element those two are
 * geometry rather than a size. The wrapper's own attributes are on the `<svg>` tag, which
 * icon-nodes.json does not contain, so there is nothing else to remove.
 */
const WRAPPER_ATTRIBUTES = new Set(['class']);

/**
 * No attribute may name a colour. `currentColor` is the whole contract - an icon takes the colour
 * of the text it sits in - and rule 15 says a colour lives in tokens.json and nowhere else. An
 * upstream icon that shipped a literal hex fill would slip past `lint-no-literals`, which does not
 * read generated output, so it is caught at the source instead. `currentColor` is not a colour in
 * this sense - it is the contract.
 */
const COLOUR = /#[0-9a-fA-F]{3,8}\b|\b(?:rgba?|hsla?)\(/;

/**
 * render builds the file and returns it without writing. Split from `generate` so the test can
 * compare the committed output against a fresh render - the check `LabelTokens.go` gets from
 * `make generate` producing no diff, which the icons would otherwise not have.
 */
export function render() {
  if (!fs.existsSync(SOURCE)) {
    throw new Error(
      `${path.relative(packageRoot, SOURCE)} is missing - run \`pnpm install\` in the workspace. ` +
        'The generated src/icons/base.ts is committed, so this is only needed to change the set.',
    );
  }
  const nodes = JSON.parse(fs.readFileSync(SOURCE, 'utf8'));

  const lines = [];
  let icons = 0;
  let bytes = 0;

  for (const [group, names] of Object.entries(DECLARED)) {
    lines.push(`\n  // ${group}`);
    for (const name of [...names].sort()) {
      const icon = nodes[name];
      // Loudly, not silently: a name Lucide dropped in an upgrade has to stop the build, or the
      // glyph quietly disappears from a control that had one yesterday.
      if (!icon) throw new Error(`lucide has no icon called "${name}" - check the name in build/icons.js`);

      const cleaned = icon.map(([tag, attributes]) => {
        const kept = Object.entries(attributes).filter(([key]) => !WRAPPER_ATTRIBUTES.has(key));
        for (const [key, value] of kept) {
          if (COLOUR.test(String(value))) {
            throw new Error(`lucide's "${name}" carries a colour in ${key}="${value}" - see rule 15`);
          }
        }
        return [tag, Object.fromEntries(kept)];
      });
      const serialised = JSON.stringify(cleaned);
      bytes += serialised.length;
      icons += 1;
      lines.push(`  '${name}': ${serialised},`);
    }
  }

  const version = JSON.parse(
    fs.readFileSync(path.join(packageRoot, 'node_modules', 'lucide-static', 'package.json'), 'utf8'),
  ).version;

  const file = `// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The declared subset of Lucide, as icon nodes.
 *
 * Generated from lucide-static@${version} by build/icons.js - DO NOT EDIT.
 * Run \`make icons\` after changing the list there. Lucide is ISC; the notice is in
 * THIRD-PARTY-LICENSES.md (ADR-0041).
 *
 * ${icons} icons, ${bytes} bytes of node data.
 */

import type { IconNode } from './node.ts';

export const BASE_ICONS = {${lines.join('\n')}
} as const satisfies Record<string, readonly IconNode[]>;
`;

  return { file, icons, bytes, version };
}

export function generate() {
  const result = render();
  fs.mkdirSync(path.dirname(DESTINATION), { recursive: true });
  fs.writeFileSync(DESTINATION, result.file);
  return result;
}

/** Where the generated file lives, for the test that checks it is in step. */
export const OUTPUT = DESTINATION;

/** Whether the source is installed. It is a devDependency, so a consumer's checkout has no it. */
export const sourceAvailable = () => fs.existsSync(SOURCE);

// Only when run, never when imported: the test reads DECLARED to check the committed output is in
// step, and a test that regenerated the file as a side effect of importing it would always pass.
if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const result = generate();
  console.log(
    `icons\n✔︎ src/icons/base.ts (${result.icons} icons from lucide-static@${result.version}, ` +
      `${result.bytes} bytes of node data)`,
  );
}
