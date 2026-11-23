// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Generates the three targets of ADR-0029 from tokens/tokens.json.
//
// Two of them land in dist/ and are ignored by git. The third is written straight into the Go
// core and IS committed, because `go build ./...` has to work for somebody who has never installed
// Node.js - and because that makes a drift between the two visible as a diff rather than as a
// rendering bug.

import { fileURLToPath } from 'node:url';
import path from 'node:path';
import StyleDictionary from 'style-dictionary';

import { cssFormat, tsFormat, goFormat } from './formats.js';
import { buildFonts } from './fonts.js';
import { buildMark } from './mark.js';

const packageRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const repositoryRoot = path.resolve(packageRoot, '..', '..');

// Where the Go artefact goes, and why it is not in dist/: validating a label colour is domain
// validation, so it belongs where project-structure.md §1 puts colour-token handling. A .go file
// under packages/ would be compiled by `go build ./...` while ADR-0027 says nothing here is
// importable from Go - a contradiction the file system would have to be trusted to keep.
const GO_DESTINATION = path.join('core', 'domain', 'model', 'shared');

StyleDictionary.registerFormat({ name: 'hubtask/css', format: cssFormat });
StyleDictionary.registerFormat({ name: 'hubtask/ts', format: tsFormat });
StyleDictionary.registerFormat({ name: 'hubtask/go', format: goFormat });

// The css group less time/seconds: durations stay in milliseconds, as the motion module and the
// stylesheets read them. style-dictionary 5.8 widened time/seconds to `duration` tokens.
const TRANSFORMS = StyleDictionary.hooks.transformGroups.css.filter((name) => name !== 'time/seconds');

const dictionary = new StyleDictionary({
  source: [path.join(packageRoot, 'tokens', 'tokens.json')],
  usesDtcg: true,
  log: { verbosity: 'default', warnings: 'warn' },
  platforms: {
    css: {
      transforms: TRANSFORMS,
      buildPath: `${path.join(packageRoot, 'dist')}${path.sep}`,
      files: [{ destination: 'tokens.css', format: 'hubtask/css' }],
    },
    ts: {
      transforms: TRANSFORMS, // the same string values; a TS consumer wants what CSS wants
      buildPath: `${path.join(packageRoot, 'dist')}${path.sep}`,
      files: [{ destination: 'tokens.ts', format: 'hubtask/ts' }],
    },
    go: {
      transforms: TRANSFORMS,
      buildPath: `${path.join(repositoryRoot, GO_DESTINATION)}${path.sep}`,
      files: [{ destination: 'LabelTokens.go', format: 'hubtask/go' }],
    },
  },
});

await dictionary.hasInitialized;
await dictionary.buildAllPlatforms();

const fonts = buildFonts();
console.log(`\nfonts\n✔︎ dist/fonts.css, dist/fonts-scripts.css (${fonts.faces} faces, ${fonts.files} files in dist/fonts/)`);

const marks = await buildMark();
console.log(`\nmark\n✔︎ dist/mark/: ${marks.join(', ')}`);
