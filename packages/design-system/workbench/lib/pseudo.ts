// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Rule 4, made observable.
//
// `design-system.md` §6: "Everything grows by 40 %. German, Finnish and Russian break any layout
// measured against English." That rule is checked once, at the moment somebody looks at a longer
// string, and never afterwards - so the workbench produces the longer string itself.
//
// Why a DOM transform rather than a locale. There is no message catalogue in the client yet
// (F1-07), and waiting for one would mean rule 4 goes unchecked through two waves of components.
// A transform over rendered text needs no cooperation from a component, which is also its
// weakness: it cannot know that a string is an identifier. `data-workbench-verbatim` is the
// escape hatch for that, and it is meant to be rare.

/**
 * Accented look-alikes. The text has to stay readable - an unreadable pseudo-locale gets switched
 * off, and an axis nobody switches on measures nothing.
 */
const ACCENTS: Record<string, string> = {
  a: 'á', b: 'ḅ', c: 'ç', d: 'ḍ', e: 'é', f: 'ƒ', g: 'ǧ', h: 'ḥ', i: 'í', j: 'ĵ', k: 'ķ', l: 'ļ',
  m: 'ṃ', n: 'ñ', o: 'ó', p: 'ṗ', q: 'q', r: 'ř', s: 'š', t: 'ţ', u: 'ú', v: 'ṿ', w: 'ŵ', x: 'ẋ',
  y: 'ý', z: 'ž',
  A: 'Á', B: 'Ḅ', C: 'Ç', D: 'Ḍ', E: 'É', F: 'Ƒ', G: 'Ǧ', H: 'Ḥ', I: 'Í', J: 'Ĵ', K: 'Ķ', L: 'Ļ',
  M: 'Ṃ', N: 'Ñ', O: 'Ó', P: 'Ṗ', Q: 'Q', R: 'Ř', S: 'Š', T: 'Ţ', U: 'Ú', V: 'Ṿ', W: 'Ŵ', X: 'Ẋ',
  Y: 'Ý', Z: 'Ž',
};

/** The growth rule 4 names. Not a round 1.5: the number in the specification is 40 %. */
const GROWTH = 1.4;

/**
 * Brackets, so that a clipped string is visibly clipped rather than merely short. A layout that
 * eats the closing bracket has eaten a word in Finnish.
 */
const OPEN = '⟦';
const CLOSE = '⟧';

/** The padding. Middle dots, because they measure width without pretending to be language. */
const PAD = '·';

export function pseudo(text: string): string {
  const trimmed = text.trim();
  if (trimmed === '') return text;

  const accented = [...text].map((character) => ACCENTS[character] ?? character).join('');
  const target = Math.ceil(trimmed.length * GROWTH);
  const padding = PAD.repeat(Math.max(1, target - trimmed.length));
  return `${OPEN}${accented}${padding}${CLOSE}`;
}

/** Attributes a person reads, and that therefore have to grow with everything else. */
const ATTRIBUTES = ['placeholder', 'title', 'aria-label', 'aria-placeholder'];

const SKIP_TAGS = new Set(['SCRIPT', 'STYLE', 'SVG', 'CODE', 'KBD', 'SAMP']);
const VERBATIM = '[data-workbench-verbatim]';

/**
 * Rewrites the visible text under `root` in place. One shot, no observer: the stage re-creates
 * the story when the axis changes, so there is never a half-transformed tree to keep in sync.
 */
export function applyPseudo(root: HTMLElement): void {
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
    acceptNode(node) {
      const parent = node.parentElement;
      if (!parent || SKIP_TAGS.has(parent.tagName) || parent.closest(VERBATIM)) {
        return NodeFilter.FILTER_REJECT;
      }
      return node.nodeValue && node.nodeValue.trim() !== ''
        ? NodeFilter.FILTER_ACCEPT
        : NodeFilter.FILTER_REJECT;
    },
  });

  const texts: Text[] = [];
  for (let node = walker.nextNode(); node; node = walker.nextNode()) texts.push(node as Text);
  for (const node of texts) node.nodeValue = pseudo(node.nodeValue ?? '');

  for (const element of root.querySelectorAll<HTMLElement>('*')) {
    if (element.closest(VERBATIM)) continue;
    for (const attribute of ATTRIBUTES) {
      const value = element.getAttribute(attribute);
      if (value) element.setAttribute(attribute, pseudo(value));
    }
    // A value is data, not copy - except in a control whose value *is* the copy on screen.
    if (element instanceof HTMLInputElement && element.type === 'text' && element.value) {
      element.value = pseudo(element.value);
    }
  }
}
