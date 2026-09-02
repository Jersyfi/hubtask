// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// What an icon is, in this package: a list of SVG child elements and their attributes.
//
// Not a string of markup. `{@html}` would be shorter and is what most icon libraries do, but it
// puts a markup parser on the path of every icon in the product for no gain - the shapes are known
// at build time, so they can be data that Svelte renders as elements. The type is also what lets
// the no-literals lint see an icon at all: a `d` attribute is geometry, a `fill` would be a colour,
// and one of those is a value tokens.json has an opinion about.

/** `['path', { d: 'M20 6 9 17l-5-5' }]` — the tag, and the attributes it carries. */
export type IconNode = readonly [tag: string, attributes: Readonly<Record<string, string>>];
