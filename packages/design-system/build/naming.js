// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The one place that decides what a token is called in each target (design-system.md §5).
//
// The names are short on purpose - `--n-500`, not `--primitive-color-neutral-500`. They are typed
// by hand hundreds of times a day, and the group a token sits in is already visible from its
// prefix. That the mapping is a table rather than a rule is deliberate too: a rule that has to
// cover `fontSize` -> `fs` and `neutral` -> `n` is a rule with exceptions, and a table with
// exceptions is just a table.

/** Prefix per primitive group. A group missing here is a mistake, not a default. */
export const PRIMITIVE_PREFIX = {
  color: null, // the family name carries it: color.blue.500 -> blue-500
  space: 'sp',
  radius: 'r',
  borderWidth: 'bw',
  layer: 'z',
  fontFamily: 'font',
  fontWeight: 'fw',
  fontSize: 'fs',
  lineHeight: 'lh',
  duration: 'dur',
  easing: 'ease',
  breakpoint: 'bp',
};

/** Colour families whose token name is shorter than their source name. */
export const FAMILY_ALIAS = { neutral: 'n' };

/** Semantic groups whose token name differs from the group in the source. */
export const SEMANTIC_ALIAS = { elevation: 'shadow' };

/**
 * cssName turns a token path into the custom property name, without the leading `--`.
 * `semantic.light.*` and `semantic.dark.*` produce the same name by design: the two modes
 * declare the same vocabulary and differ only in what it resolves to.
 */
export function cssName(path) {
  const [root, group, ...rest] = path;
  if (root === 'primitive') {
    if (group === 'color') {
      const [family, ...step] = rest;
      return [FAMILY_ALIAS[family] ?? family, ...step].join('-');
    }
    const prefix = PRIMITIVE_PREFIX[group];
    if (prefix === undefined) {
      throw new Error(`no name prefix for the primitive group "${group}" (build/naming.js)`);
    }
    return [prefix, ...rest].join('-');
  }
  if (root === 'semantic') {
    const [, , semanticGroup, ...tail] = path; // path[1] is the mode, which the name drops
    return [SEMANTIC_ALIAS[semanticGroup] ?? semanticGroup, ...tail].join('-');
  }
  throw new Error(`unexpected token root "${root}" in ${path.join('.')}`);
}

/** camel turns a kebab segment into the camelCase a TypeScript consumer expects. */
export function camel(segment) {
  return segment.replace(/-([a-z0-9])/g, (_, c) => c.toUpperCase());
}

/** pascal is the Go exported-identifier spelling of a token name. */
export function pascal(segment) {
  const c = camel(segment);
  return c.charAt(0).toUpperCase() + c.slice(1);
}
