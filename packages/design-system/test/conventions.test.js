// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The rules of design-system.md §5 and §6, as a gate rather than as a review comment.
//
// F1-05's brief says every component arrives with a test, "because a component with no test is one
// the next refactor breaks silently". What that test should assert is the question, and the answer
// this file gives is: the rules that hold for all of them, over the source, in plain Node.
//
// Not a rendering test. Rendering forty components would need a DOM, a compiler and a runner -
// three dependencies for a supply chain that has counted every one it has (ADR-0037), to check
// things the workbench already shows a person better than an assertion can. What a person cannot
// check by looking is whether the rule held in *every* component, every time, which is exactly
// what a reader of source is good at.
//
// The failure each rule guards against is named in its own test, because a gate whose message does
// not say what went wrong is a gate people delete.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const packageRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const sourceDir = path.join(packageRoot, 'src');

/** Every `.svelte` under src/, parts included: a rule that a part may break is not a rule. */
function components() {
  const out = [];
  const walk = (dir) => {
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
      const joined = path.join(dir, entry.name);
      if (entry.isDirectory()) walk(joined);
      else if (entry.name.endsWith('.svelte')) {
        out.push({ name: entry.name, relative: path.relative(packageRoot, joined), source: fs.readFileSync(joined, 'utf8') });
      }
    }
  };
  walk(sourceDir);
  return out;
}

const ALL = components();
/**
 * The style block with its comments removed. Stripping them is not a convenience: the rules below
 * are about what the browser is told, and the first thing this gate found was its own comment
 * saying "never left/right".
 */
const styleOf = (source) => source.slice(source.indexOf('<style>')).replace(/\/\*[\s\S]*?\*\//g, '');
const scriptOf = (source) => source.slice(0, source.indexOf('</script>'));

/** A component that owns an interactive element, as opposed to a layout primitive or a demo. */
const INTERACTIVE = /<(button|input|select|textarea|a\s)/;

test('src/ is not empty and every file was read', () => {
  assert.ok(ALL.length >= 10, `only ${ALL.length} components found - the walk is broken`);
});

test('no component writes left or right', () => {
  // §3: alignment is start/end only, because RTL is a requirement and not a later port. The
  // failure is invisible in development and total in Arabic, which is why it is a gate.
  const physical =
    /\b(?:margin|padding|border|inset)?-?(?:left|right)\s*:|:\s*(?:left|right)\s*[;}]|\btext-align\s*:\s*(?:left|right)\b/;
  for (const component of ALL) {
    const style = styleOf(component.source);
    const match = physical.exec(style);
    assert.equal(
      match,
      null,
      `${component.relative} uses ${match?.[0]?.trim()}. Use the logical property - inline-start, ` +
        'inline-end, or text-align: start (design-system.md §3).',
    );
  }
});

test('every interactive component has a visible focus ring', () => {
  // Rule 5: "the app is fully operable by keyboard or it is not". One control out of eight with no
  // ring is the failure this catches, and it is the one nobody notices with a mouse.
  for (const component of ALL) {
    if (!INTERACTIVE.test(component.source)) continue;
    if (component.name.startsWith('_')) continue; // a part draws no ring of its own
    const style = styleOf(component.source);
    assert.match(
      style,
      /:focus-visible/,
      `${component.relative} renders a control and has no :focus-visible rule (rule 5)`,
    );
    assert.match(
      style,
      /outline:\s*var\(--bw-ring\)\s+solid\s+var\(--focus-ring\)/,
      `${component.relative} draws a focus ring that is not rule 5's: 2 px, --focus-ring`,
    );
    assert.match(style, /outline-offset:\s*var\(--sp-025\)/, `${component.relative} misses rule 5's 2 px offset`);
  }
});

test('states are CSS states, never props', () => {
  // §5: a variant matrix that contains states explodes. The moment `hover` is a prop, every other
  // prop has to be crossed with it.
  const forbidden = /^\s*(?:is)?(?:hover|hovered|pressed|active|focused|focus)\??\s*:/im;
  for (const component of ALL) {
    const script = scriptOf(component.source);
    const match = forbidden.exec(script);
    assert.equal(
      match,
      null,
      `${component.relative} declares a prop ${match?.[0]?.trim()} - hover, pressed and focus are ` +
        'CSS states (design-system.md §5)',
    );
  }
});

test('a control that can be switched off carries its reason', () => {
  // The CapabilityGate principle one level down: ErrCapabilityNotSupported must never become
  // silent ignoring. There is no `disabled` boolean anywhere - `disabledReason` is what disables,
  // so the two cannot come apart.
  for (const component of ALL) {
    const script = scriptOf(component.source);
    assert.doesNotMatch(
      script,
      /^\s*disabled\??\s*:\s*boolean/im,
      `${component.relative} takes a bare \`disabled\` boolean. Use \`disabledReason\`: a control ` +
        'the reader cannot use owes them the reason (design-system.md §4, CapabilityGate).',
    );
    // Where a control *is* disabled, the reason has to reach the accessibility tree.
    if (/disabled=\{[^}]*disabledReason/.test(component.source)) {
      assert.match(
        component.source,
        /aria-describedby/,
        `${component.relative} disables a control without pointing at the reason with aria-describedby`,
      );
    }
  }
});

test('motion is confined to opacity and transform', () => {
  // Rule 6. Layout is never animated: a transition on width or padding moves everything beside it,
  // and on a slow machine it moves it visibly.
  const animatable = /transition:\s*([^;]+);/g;
  const allowed = /^(?:opacity|transform|translate|rotate|scale|background-color|color|box-shadow|border-color|outline-color|none)$/;
  for (const component of ALL) {
    for (const match of styleOf(component.source).matchAll(animatable)) {
      for (const part of match[1].split(',')) {
        const property = part.trim().split(/\s+/)[0];
        assert.match(
          property,
          allowed,
          `${component.relative} animates ${property}. Rule 6: opacity and transform only - a ` +
            'colour or a shadow does not move anything, a length does.',
        );
      }
    }
  }
});

test('every component that moves honours a reduced-motion preference', () => {
  // Rule 6 again, and its second half: the attribute *alongside* the media query, because §7 needs
  // a user preference and a preference only the operating system can set is not one (ADR-0037).
  for (const component of ALL) {
    const style = styleOf(component.source);
    if (!/transition:|animation:/.test(style)) continue;
    assert.match(
      style,
      /@media \(prefers-reduced-motion: reduce\)/,
      `${component.relative} animates and does not answer prefers-reduced-motion (rule 6)`,
    );
    assert.match(
      style,
      /\[data-motion='reduced'\]/,
      `${component.relative} answers the media query but not [data-motion="reduced"] (rule 6, ADR-0037)`,
    );
  }
});

/**
 * Names the platform owns. `checked` is a checkbox's *value*, the way `value` is an input's, and
 * renaming it would break `bind:checked` and disagree with the element underneath. §5's rule is
 * about the booleans we invent.
 */
const PLATFORM_BOOLEANS = new Set(['checked', 'indeterminate', 'required', 'readonly', 'multiple', 'open', 'selected']);

test('a boolean prop we invented asks a question', () => {
  // §5: `size`, `tone`, `isDisabled`, `hasIcon`. A boolean called `busy` reads as a noun at the
  // call site and its opposite is unspellable.
  const booleans = /^\s*(\w+)\??\s*:\s*boolean/gim;
  for (const component of ALL) {
    for (const match of scriptOf(component.source).matchAll(booleans)) {
      const name = match[1];
      if (PLATFORM_BOOLEANS.has(name)) continue;
      assert.match(
        name,
        /^(?:is|has|can|should)[A-Z]/,
        `${component.relative} has a boolean prop \`${name}\`. §5: a boolean asks a question - ` +
          `\`is${name[0].toUpperCase()}${name.slice(1)}\`.`,
      );
    }
  }
});

test('the picture of a control never takes the click meant for it', () => {
  // The pattern is the same in Checkbox, Radio and Switch: a transparent native input on top, and
  // a painted part beside it that shows the state. The painting has to be inert, and "the input is
  // positioned so it paints above" is not enough - a painted part that is *translated* creates a
  // stacking context of its own and is painted in the same layer, later. That is exactly what
  // happened to Switch: with the knob moved to the checked position it covered the input, and the
  // switch could only be turned off by clicking the track beside its own knob.
  for (const component of ALL) {
    const style = styleOf(component.source);
    if (!/\.native\s*\{/.test(style)) continue;
    assert.match(
      style,
      /pointer-events:\s*none/,
      `${component.relative} paints a control over a native input without making the painting ` +
        'inert. Add `pointer-events: none` to it: the input is the control, this is a picture of it.',
    );
  }
});

test('no component hides a control from the accessibility tree', () => {
  // The Checkbox, Radio and Switch pattern: the native input is transparent and on top, never
  // `display: none`, which would take the keyboard and the screen reader with it.
  for (const component of ALL) {
    const style = styleOf(component.source);
    for (const match of style.matchAll(/\.native[^{]*\{([^}]*)\}/g)) {
      assert.doesNotMatch(
        match[1],
        /display:\s*none|visibility:\s*hidden/,
        `${component.relative} hides its native input, which removes it from the accessibility tree`,
      );
    }
  }
});
