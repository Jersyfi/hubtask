// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Writes the mark's two standalone files into dist/mark/, from src/mark.ts and the resolved
// brand.* values of tokens.json - never from a colour written here:
//
//   favicon.svg   the reduced mark, the browser tab's icon (UC-WRK-33 check 4). A file loaded as
//                 an image cannot read a custom property, so its colours are the resolved values:
//                 the light ones as presentation attributes, the dark ones in a single media rule.
//                 An engine that ignored the rule would still draw the light mark, never an
//                 unstyled one.
//   app-icon.svg  the source of the installed apps' icons: the full mark on the brand tile, one
//                 form for light and dark launchers alike.
//
// Generated rather than committed, so the website's old hand-copied favicon literals have no
// successor that could drift from tokens.json.

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

import { FULL_MARK, MARK_GRID, REDUCED_MARK, squarePath } from '../src/mark.ts';

const packageRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
export const MARK_OUTPUT = path.join(packageRoot, 'dist', 'mark');

/** The share of the app icon's square the mark takes (design-system.md §9). */
const APP_ICON_MARK_SHARE = 0.72;
/** The app icon's corner, as a share of its side - a rounded square, which every launcher masks further. */
const APP_ICON_CORNER_SHARE = 0.225;

/** favicon renders the tab icon from the light and the dark brand values. */
export function favicon(light, dark) {
  const form = REDUCED_MARK;
  const { cx, cy, r } = form.spark;
  return [
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${MARK_GRID} ${MARK_GRID}">`,
    `<style>@media (prefers-color-scheme: dark){.outline{stroke:${dark.outline}}.plane{fill:${dark.plane}}.spark{fill:${dark.spark}}}</style>`,
    `<path class="outline" d="${form.outline.d}" fill="none" stroke="${light.outline}" stroke-width="${form.outline.stroke}" stroke-linecap="round"/>`,
    `<path class="plane" d="${squarePath(form.plane)}" fill="${light.plane}"/>`,
    `<circle class="spark" cx="${cx}" cy="${cy}" r="${r}" fill="${light.spark}"/>`,
    '</svg>',
    '',
  ].join('\n');
}

/** appIcon renders the installed apps' icon source: the full mark on the tile. */
export function appIcon(brand) {
  const form = FULL_MARK;
  const inset = (MARK_GRID * (1 - APP_ICON_MARK_SHARE)) / 2;
  const { cx, cy, r } = form.spark;
  return [
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${MARK_GRID} ${MARK_GRID}">`,
    `<rect width="${MARK_GRID}" height="${MARK_GRID}" rx="${MARK_GRID * APP_ICON_CORNER_SHARE}" fill="${brand.tile}"/>`,
    `<g transform="translate(${inset} ${inset}) scale(${APP_ICON_MARK_SHARE})">`,
    `<path d="${form.outline.d}" fill="none" stroke="${brand.tileOutline}" stroke-width="${form.outline.stroke}" stroke-linecap="round"/>`,
    `<path d="${squarePath(form.plane)}" fill="${brand.plane}"/>`,
    `<path d="${squarePath(form.core)}" fill="${brand.core}"/>`,
    `<circle cx="${cx}" cy="${cy}" r="${r}" fill="${brand.spark}"/>`,
    '</g>',
    '</svg>',
    '',
  ].join('\n');
}

/** buildMark writes both files; `dist/tokens.ts` must exist, which the token build sees to. */
export async function buildMark() {
  const { values } = await import(pathToFileURL(path.join(packageRoot, 'dist', 'tokens.ts')).href);
  fs.mkdirSync(MARK_OUTPUT, { recursive: true });
  fs.writeFileSync(path.join(MARK_OUTPUT, 'favicon.svg'), favicon(values.light.brand, values.dark.brand));
  fs.writeFileSync(path.join(MARK_OUTPUT, 'app-icon.svg'), appIcon(values.light.brand));
  return ['favicon.svg', 'app-icon.svg'];
}
