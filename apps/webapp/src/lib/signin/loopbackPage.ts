// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * How the app's loopback page looks (UC-SYN-07/3, *Back to the app*): the sign-in
 * card's look, handed to the shell with the words, because the page is served by the shell and can
 * load nothing - its policy is `default-src 'none'`. The values are this page's own tokens as the
 * browser resolved them, so the card is in the theme the app is in and no value is written twice
 * (rule 15); the mark is the one this page draws, its colours set on it.
 */

/** The tokens the card reads, copied as resolved. */
const PROPERTIES = [
  '--bg-canvas', '--bg-surface', '--border-subtle', '--text-primary', '--text-secondary',
  '--ambient-signature', '--ambient-primary', '--shadow-raised', '--r-lg', '--bw-hairline',
  '--sp-050', '--sp-100', '--sp-200', '--sp-250', '--sp-300', '--sp-400',
  '--font-ui', '--font-display', '--fs-100', '--fs-400', '--fs-500',
  '--fw-semibold', '--fw-bold', '--lh-tight', '--lh-normal',
] as const;

/** The card's structure; every value a token. The same geometry as `SignInCard.svelte`. */
const LAYOUT = [
  'html{background:var(--bg-canvas)}',
  'body{margin:0;min-height:100vh;box-sizing:border-box;display:flex;align-items:center;justify-content:center;',
  'padding:var(--sp-400) var(--sp-200);color:var(--text-primary);font-family:var(--font-ui);font-size:var(--fs-100);line-height:var(--lh-normal);',
  'background:radial-gradient(ellipse 70% 60% at 12% -10%,var(--ambient-signature),transparent 70%),',
  'radial-gradient(ellipse 80% 70% at 100% 100%,var(--ambient-primary),transparent 70%),var(--bg-canvas)}',
  'main{box-sizing:border-box;inline-size:min(100%,52ch);display:grid;gap:var(--sp-250);padding:var(--sp-300);',
  'background:var(--bg-surface);border:var(--bw-hairline) solid var(--border-subtle);border-radius:var(--r-lg);box-shadow:var(--shadow-raised)}',
  '.logo{display:inline-flex;align-items:center;gap:var(--sp-100)}',
  '.logo svg{inline-size:var(--sp-300);block-size:var(--sp-300);overflow:visible}',
  '.wordmark{font-family:var(--font-display);font-weight:var(--fw-bold);font-size:var(--fs-400);line-height:1;text-transform:lowercase}',
  '.text{display:grid;gap:var(--sp-050)}',
  'h1{margin:0;font-family:var(--font-display);font-size:var(--fs-500);font-weight:var(--fw-semibold);line-height:var(--lh-tight)}',
  'p{margin:0;color:var(--text-secondary)}',
].join('');

/** The stylesheet: the resolved tokens, then the structure. Nothing in it can close the element. */
export function loopbackStyle(read: (property: string) => string): string {
  const values = PROPERTIES.map((property) => [property, read(property).trim()] as const)
    .filter(([, value]) => value !== '' && !/[<>{};]/.test(value))
    .map(([property, value]) => `${property}:${value}`)
    .join(';');
  return `:root{${values}}${LAYOUT}`;
}

/** The mark this page draws, with its colours on it rather than in a stylesheet the shell lacks. */
export function loopbackMark(doc: Document): string {
  const drawn = doc.querySelector<SVGSVGElement>('.logo svg');
  if (drawn === null) return '';
  const copy = drawn.cloneNode(true) as SVGSVGElement;
  const originals = drawn.querySelectorAll<SVGElement>('path, circle');
  copy.querySelectorAll<SVGElement>('path, circle').forEach((part, index) => {
    const original = originals[index];
    if (original === undefined) return;
    const style = getComputedStyle(original);
    part.setAttribute('fill', style.fill);
    part.setAttribute('stroke', style.stroke);
    part.removeAttribute('class');
  });
  copy.removeAttribute('class');
  return copy.outerHTML;
}

/** What the shell is handed besides the words. */
export function loopbackLook(doc: Document = document): { readonly style: string; readonly mark: string } {
  const computed = getComputedStyle(doc.documentElement);
  return { style: loopbackStyle((property) => computed.getPropertyValue(property)), mark: loopbackMark(doc) };
}
