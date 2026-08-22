// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Collects the IBM Plex faces into dist/, so that a Hubtask contacts no foreign domain when it
// loads (design-system.md §3, ADR-0029). Nothing is fetched at build time either: the faces come
// from the packages the lockfile pins, and this only copies and rewrites them.
//
// Every subset each package ships is kept. `unicode-range` means a browser downloads only the
// subsets it actually needs, so keeping Cyrillic and Greek costs a reader nothing and spares us
// discovering at the wrong moment that rule 4 of design-system.md meant Russian literally.

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const packageRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const modules = path.join(packageRoot, 'node_modules');

// The three cuts of design-system.md §3, and the stylesheets each one needs.
//
// IBM Plex Sans is taken as a variable font because `body` is weight 450 - a weight no static
// instance has, and the reason the variable release is not optional here. Fontsource registers it
// under "IBM Plex Sans Variable"; it is renamed so that the family in tokens.json stays the name
// of the typeface rather than the name of a distribution format.
const SOURCES = [
  {
    package: '@fontsource-variable/ibm-plex-sans',
    stylesheets: ['wght.css', 'wght-italic.css'],
    rename: { 'IBM Plex Sans Variable': 'IBM Plex Sans' },
  },
  { package: '@fontsource/ibm-plex-sans-condensed', stylesheets: ['600.css', '700.css'] },
  { package: '@fontsource/ibm-plex-mono', stylesheets: ['400.css', '400-italic.css', '500.css'] },
];

const BANNER = `/**
 * IBM Plex, self-hosted.
 *
 * Assembled from the @fontsource packages by build/fonts.js - DO NOT EDIT.
 * The typeface is licensed OFL-1.1; see THIRD-PARTY-LICENSES.md.
 */`;

export function buildFonts() {
  const outCss = path.join(packageRoot, 'dist', 'fonts.css');
  const outFiles = path.join(packageRoot, 'dist', 'fonts');
  fs.rmSync(outFiles, { recursive: true, force: true });
  fs.mkdirSync(outFiles, { recursive: true });

  const copied = new Set();
  const blocks = [];

  for (const source of SOURCES) {
    const base = path.join(modules, source.package);
    if (!fs.existsSync(base)) {
      throw new Error(`${source.package} is not installed - run \`pnpm install\` first`);
    }
    for (const stylesheet of source.stylesheets) {
      let css = fs.readFileSync(path.join(base, stylesheet), 'utf8');

      // woff2 only. Every browser that can run this application has supported it for years, and
      // carrying the woff fallback doubles the number of files for nobody.
      css = css.replace(/,\s*url\(\.\/files\/[^)]+\.woff\)\s*format\('woff'\)/g, '');

      for (const [from, to] of Object.entries(source.rename ?? {})) {
        css = css.replaceAll(`'${from}'`, `'${to}'`);
      }

      css = css.replace(/url\(\.\/files\/([^)]+)\)/g, (_, file) => {
        if (!copied.has(file)) {
          fs.copyFileSync(path.join(base, 'files', file), path.join(outFiles, file));
          copied.add(file);
        }
        return `url(./fonts/${file})`;
      });

      blocks.push(css.trim());
    }
  }

  fs.writeFileSync(outCss, `${BANNER}\n\n${blocks.join('\n\n')}\n`);
  return { faces: blocks.join('\n').match(/@font-face/g)?.length ?? 0, files: copied.size };
}
