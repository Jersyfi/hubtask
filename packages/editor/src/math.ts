// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A formula block, `$$` … `$$`: its TeX is the block's attribute, drawn by KaTeX as MathML.
 *
 * **MathML, not KaTeX's HTML.** The HTML output needs KaTeX's style sheet, its fonts and inline
 * style attributes the content security policy refuses; MathML is drawn by the browser itself.
 * KaTeX is loaded on the first formula a page shows, never with the editor (the first-screen budget),
 * and with `trust` off, so no `\href` or `\includegraphics` reaches the page.
 */

import { Node, mergeAttributes, type Editor } from '@tiptap/core';

import { blockLibraries } from './libraries.ts';
import { texOf } from './markdown.ts';


/** The words the formula block draws: its label and the line shown for TeX KaTeX cannot read. */
export interface MathWords {
  words?(key: 'math' | 'math_error'): string;
}

type Katex = typeof import('katex').default;

let katex: Promise<Katex> | undefined;

/** KaTeX, loaded once, on demand. */
export function loadKatex(): Promise<Katex> {
  katex ??= blockLibraries.katex().then((module) => module.default);
  return katex;
}

/** A formula as MathML, or nothing for TeX KaTeX refuses. Bounded, so a formula cannot stall the page. */
export function mathMarkup(engine: Katex, tex: string): string | undefined {
  try {
    return engine.renderToString(tex, { displayMode: true, output: 'mathml', throwOnError: true, trust: false, strict: 'ignore', maxSize: 50, maxExpand: 500 });
  } catch {
    return undefined;
  }
}

function draw(into: HTMLElement, tex: string, drawing: MathWords | undefined) {
  into.replaceChildren();
  if (tex === '') return;
  void loadKatex().then((engine) => {
    const markup = mathMarkup(engine, tex);
    if (markup === undefined) {
      into.classList.add('page-math__formula--error');
      into.textContent = drawing?.words?.('math_error') ?? tex;
      return;
    }
    into.classList.remove('page-math__formula--error');
    // KaTeX's own output, from TeX with `trust` off: markup the library builds, never the page's text.
    into.innerHTML = markup;
  }, () => {
    into.textContent = tex;
  });
}

/** The formula block. A writer edits the TeX under the drawn formula; a reader sees the formula. */
export const MathBlock = Node.create<{ drawing?: MathWords }>({
  name: 'mathBlock',
  group: 'block',
  atom: true,
  selectable: true,
  draggable: true,
  addOptions: () => ({ drawing: undefined }),
  addAttributes: () => ({ latex: { default: '' } }),
  parseHTML: () => [{ tag: 'div[data-math]', getAttrs: (dom: HTMLElement) => ({ latex: texOf(dom.dataset['math'] ?? '') }) }],
  renderHTML: ({ node, HTMLAttributes }) => ['div', mergeAttributes(HTMLAttributes, { 'data-math': texOf(node.attrs['latex']), class: 'page-math' })],
  addNodeView() {
    const drawing = this.options.drawing;
    return (({ node, editor, getPos }: { node: { attrs: Record<string, unknown> }; editor: Editor; getPos: () => number | undefined }) => {
      const dom = document.createElement('div');
      dom.className = 'page-math';
      dom.setAttribute('contenteditable', 'false');
      const formula = document.createElement('div');
      formula.className = 'page-math__formula';
      formula.setAttribute('role', 'math');
      formula.setAttribute('aria-label', texOf(node.attrs['latex']));
      dom.append(formula);
      draw(formula, texOf(node.attrs['latex']), drawing);
      if (editor.isEditable) {
        const source = document.createElement('textarea');
        source.className = 'page-math__source';
        source.value = texOf(node.attrs['latex']);
        source.spellcheck = false;
        source.rows = Math.min(8, Math.max(1, source.value.split('\n').length));
        source.setAttribute('aria-label', drawing?.words?.('math') ?? '');
        source.addEventListener('keydown', (event) => event.stopPropagation());
        source.addEventListener('input', () => draw(formula, texOf(source.value), drawing));
        source.addEventListener('change', () => {
          const at = getPos();
          if (at === undefined) return;
          editor.view.dispatch(editor.state.tr.setNodeAttribute(at, 'latex', texOf(source.value)));
        });
        dom.append(source);
      }
      return {
        dom,
        stopEvent: (event: Event) => event.target instanceof HTMLTextAreaElement,
        ignoreMutation: () => true,
        update: (next: { type: { name: string }; attrs: Record<string, unknown> }) => {
          if (next.type.name !== 'mathBlock') return false;
          const tex = texOf(next.attrs['latex']);
          formula.setAttribute('aria-label', tex);
          const source = dom.querySelector('textarea');
          if (source && document.activeElement !== source) source.value = tex;
          draw(formula, tex, drawing);
          return true;
        },
      };
    }) as never;
  },
});
