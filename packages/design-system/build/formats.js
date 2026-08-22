// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The three targets of ADR-0029, as Style Dictionary formats.
//
// They are hand-written rather than assembled from the built-in formats for one reason: the names
// are short (`--n-500`, not `--primitive-color-neutral-500`) and the CSS is cut into three blocks
// that the built-in `css/variables` cannot produce from one token set. What the library still does
// is the part worth having - parsing DTCG, resolving references, and turning a shadow, a cubic
// bezier and a font stack into the string CSS wants.

import { cssName, camel, pascal } from './naming.js';

const BANNER = (what) => `/**
 * ${what}
 *
 * Generated from packages/design-system/tokens/tokens.json - DO NOT EDIT.
 * Run \`make tokens\` after changing the source. A value written anywhere but in that file is a
 * value that will disagree with itself sooner or later (ADR-0029).
 */`;

/** reference returns the token path a value is a pure alias of, or null. */
function reference(token) {
  const original = token.original?.$value;
  if (typeof original !== 'string') return null;
  const match = /^\{([^}]+)\}$/.exec(original.trim());
  return match ? match[1].split('.') : null;
}

/** cssValue prefers `var(--other)` over the resolved literal, so the cascade stays readable. */
function cssValue(token) {
  const ref = reference(token);
  return ref ? `var(--${cssName(ref)})` : String(token.$value);
}

const isPrimitive = (t) => t.path[0] === 'primitive';
const inMode = (mode) => (t) => t.path[0] === 'semantic' && t.path[1] === mode;

function declarations(tokens, indent = '  ') {
  return tokens.map((t) => `${indent}--${cssName(t.path)}: ${cssValue(t)};`).join('\n');
}

/**
 * cssFormat writes the primitives to `:root` and each semantic mode to its own `[data-theme]`
 * block, which is exactly the structure tokens.json declares. There is deliberately no fallback
 * for a document without `data-theme`: a page that forgets it should look broken immediately
 * rather than silently pick a mode nobody chose.
 */
export const cssFormat = ({ dictionary }) => {
  const all = dictionary.allTokens;
  const blocks = [
    `:root {\n${declarations(all.filter(isPrimitive))}\n}`,
    ...['light', 'dark'].map(
      (mode) => `[data-theme="${mode}"] {\n${declarations(all.filter(inMode(mode)))}\n}`,
    ),
  ];
  return `${BANNER('Hubtask design tokens as CSS custom properties.')}\n\n${blocks.join('\n\n')}\n`;
};

/** nest turns a flat token list into the nested camelCase object a TypeScript consumer reads. */
function nest(tokens, value, dropSegments) {
  const root = {};
  for (const token of tokens) {
    const path = token.path.slice(dropSegments).map(camel);
    let node = root;
    for (const segment of path.slice(0, -1)) {
      node[segment] ??= {};
      node = node[segment];
    }
    node[path.at(-1)] = value(token);
  }
  return root;
}

function literal(node, indent = '  ') {
  if (typeof node === 'string') return JSON.stringify(node);
  const inner = Object.entries(node)
    .map(([k, v]) => `${indent}  ${/^[A-Za-z_$][\w$]*$/.test(k) ? k : JSON.stringify(k)}: ${literal(v, `${indent}  `)},`)
    .join('\n');
  return `{\n${inner}\n${indent}}`;
}

/**
 * tsFormat exports the semantic layer as `var(--x)` strings rather than as colours.
 *
 * That is the whole point of the file. A component that writes the light-mode hex is a component
 * that is wrong in dark mode, and no type can catch it. Handing out the custom property instead
 * makes the theme-correct thing the only thing on offer. The resolved literals are still exported,
 * under `values`, for the few places that genuinely cannot use a custom property - a canvas, an
 * OG image, a mail template.
 */
export const tsFormat = ({ dictionary }) => {
  const all = dictionary.allTokens;
  const varOf = (t) => `var(--${cssName(t.path)})`;
  const semantic = all.filter(inMode('light')); // both modes share one vocabulary
  const parts = [
    BANNER('Hubtask design tokens for TypeScript consumers.'),
    '',
    '/** The semantic layer, as custom properties. This is what components use. */',
    `export const tokens = ${literal(nest(semantic, varOf, 2))} as const;`,
    '',
    '/** The primitive layer, as custom properties. Application code binds to `tokens` instead. */',
    `export const primitive = ${literal(nest(all.filter(isPrimitive), varOf, 1))} as const;`,
    '',
    '/** Resolved literals per mode, for the few consumers that cannot use a custom property. */',
    `export const values = ${literal({
      light: nest(all.filter(inMode('light')), (t) => String(t.$value), 2),
      dark: nest(all.filter(inMode('dark')), (t) => String(t.$value), 2),
    })} as const;`,
    '',
    "export type ThemeMode = 'light' | 'dark';",
    'export type Tokens = typeof tokens;',
    '',
    '/** The ten label colours a `colorToken` may name. The backend validates against this list. */',
    `export const labelTokens = [${labelNames(all).map((n) => `'${n}'`).join(', ')}] as const;`,
    'export type LabelToken = (typeof labelTokens)[number];',
    '',
  ];
  return parts.join('\n');
};

/** labelNames is the ten label colours, in the order the source declares them. */
function labelNames(allTokens) {
  const names = [];
  for (const token of allTokens) {
    const [root, mode, group, name] = token.path;
    if (root === 'semantic' && mode === 'light' && group === 'label' && !names.includes(name)) {
      names.push(name);
    }
  }
  return names;
}

/**
 * goFormat emits the label token NAMES and nothing else.
 *
 * Never a colour value: `domain-model.md` §4 stores a `colorToken` on `Label` and on `cover`
 * precisely so that the backend does not hold display information, and a hex constant in
 * `core/domain` would undo that. What the core needs is the vocabulary - enough to refuse a token
 * that does not exist - and that is exactly what this is (ADR-0029).
 */
export const goFormat = ({ dictionary }) => {
  const names = labelNames(dictionary.allTokens);
  // gofmt aligns a const block on its widest identifier, and `make gate-quick` fails on a file
  // gofmt would change. Emitting it already aligned keeps the generator free of a Go toolchain -
  // somebody working on the design system need not have one installed.
  const width = Math.max(...names.map((n) => `LabelToken${pascal(n)}`.length));
  const constants = names
    .map((n) => {
      const identifier = `LabelToken${pascal(n)}`;
      return `\t${identifier.padEnd(width)} LabelToken = ${JSON.stringify(n)}`;
    })
    .join('\n');
  const list = names.map((n) => `\tLabelToken${pascal(n)},`).join('\n');
  return `// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Code generated from packages/design-system/tokens/tokens.json. DO NOT EDIT.

package shared

// LabelToken is the name of a label colour. The domain stores the name, never the colour: what a
// token looks like is a question for the theme the client is rendering in, and answering it here
// would put display information in the backend (ADR-0011, ADR-0029).
type LabelToken string

// The ${names.length} label colours the design system defines.
const (
${constants}
)

// LabelTokens lists every label colour, in the order the design system declares them. A client
// that offers a colour picker gets the order from here rather than sorting the names.
var LabelTokens = []LabelToken{
${list}
}

// IsLabelToken reports whether a name is one of the label colours.
//
// The check is a membership test over a generated list rather than a hand-written switch, so that
// an eleventh colour added to tokens.json cannot be accepted by the frontend and refused here.
func IsLabelToken(name string) bool {
\tfor _, token := range LabelTokens {
\t\tif string(token) == name {
\t\t\treturn true
\t\t}
\t}
\treturn false
}
`;
};
