// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What a fenced block shows beyond its text: the colours of its language (lowlight), and for a
 * block of language `mermaid` the diagram it describes, drawn under it. The block itself stays
 * plain text, so the Markdown is the fence as written and the live document never holds a colour.
 *
 * Both libraries are loaded the first time a page holds such a block, never with the editor (the
 * first-screen budget). Until they arrive the text is drawn as it is.
 *
 * **A diagram is a picture.** Mermaid's SVG carries its own style element, which the content
 * security policy refuses inside the page; drawn as an image it is styled inside its own document
 * and runs no script. It is drawn in the colours the application hands over (the light theme's, as
 * an exported picture is), with labels as SVG text, since an image draws no HTML label.
 */

import { Extension } from '@tiptap/core';
import type { Node as PmNode } from '@tiptap/pm/model';
import { Plugin, PluginKey, type EditorState } from '@tiptap/pm/state';
import { Decoration, DecorationSet, type EditorView } from '@tiptap/pm/view';

/** The colours a diagram is drawn in. */
export interface DiagramColours {
  readonly paper: string;
  readonly ink: string;
  readonly line: string;
  readonly surface: string;
}

/** What the application hands the code blocks: a diagram's colours and the words for a broken one. */
export interface CodeDrawing {
  diagramColours?(): DiagramColours;
  words?(key: 'diagram' | 'diagram_error'): string;
}

/** One coloured run of a block's text: where it starts and ends, and its classes. */
export interface CodeRun {
  readonly from: number;
  readonly to: number;
  readonly classes: string;
}

type Lowlight = ReturnType<typeof import('lowlight').createLowlight>;
type HastNode = { type: string; value?: string; properties?: { className?: unknown }; children?: HastNode[] };

/** The runs of a highlighted tree, offsets into the text it was made from. */
export function runsOf(tree: { children: readonly HastNode[] }): CodeRun[] {
  const runs: CodeRun[] = [];
  let at = 0;
  const walk = (nodes: readonly HastNode[], classes: readonly string[]) => {
    for (const node of nodes) {
      if (node.type === 'text') {
        const length = node.value?.length ?? 0;
        if (classes.length > 0 && length > 0) runs.push({ from: at, to: at + length, classes: classes.join(' ') });
        at += length;
        continue;
      }
      const own = Array.isArray(node.properties?.className) ? (node.properties.className as unknown[]).filter((each): each is string => typeof each === 'string') : [];
      walk(node.children ?? [], [...classes, ...own]);
    }
  };
  walk(tree.children, []);
  return runs;
}

/** The runs of a block's text in its language; none for a language lowlight does not know. */
export function highlight(lowlight: Lowlight, language: string, text: string): CodeRun[] {
  if (language === '' || !lowlight.registered(language) || text.length > MAX_HIGHLIGHT) return [];
  try {
    return runsOf(lowlight.highlight(language, text) as never);
  } catch {
    return [];
  }
}

/** A block longer than this is drawn plain: colouring it on every keystroke would stall the page. */
const MAX_HIGHLIGHT = 100_000;
/** Nor is a diagram longer than this drawn. */
const MAX_DIAGRAM = 20_000;

let lowlight: Promise<Lowlight> | undefined;
let mermaid: Promise<typeof import('mermaid').default> | undefined;

function loadLowlight(): Promise<Lowlight> {
  lowlight ??= import('lowlight').then((module) => module.createLowlight(module.common));
  return lowlight;
}

function loadMermaid(colours: DiagramColours | undefined) {
  mermaid ??= import('mermaid').then((module) => {
    module.default.initialize({
      startOnLoad: false,
      securityLevel: 'strict',
      htmlLabels: false,
      flowchart: { htmlLabels: false },
      theme: 'base',
      ...(colours ? {
        themeVariables: {
          background: colours.paper, primaryColor: colours.surface, primaryTextColor: colours.ink,
          primaryBorderColor: colours.line, lineColor: colours.line, textColor: colours.ink,
        },
      } : {}),
    });
    return module.default;
  });
  return mermaid;
}

/** A diagram's picture, or why there is none. */
type Diagram = { readonly src: string } | { readonly error: true };

const diagrams = new Map<string, Diagram>();
let sequence = 0;

/** The SVG as an image's address, on a sheet of the paper colour so it reads on either theme. */
export function diagramSource(svg: string, paper: string | undefined): string {
  const sheet = paper ? svg.replace(/(<svg\b[^>]*>)/u, `$1<rect width="100%" height="100%" fill="${paper.replace(/[^#\w(),.% -]/gu, '')}"/>`) : svg;
  return `data:image/svg+xml;charset=utf-8,${encodeURIComponent(sheet)}`;
}

const key = new PluginKey<DecorationSet>('codeBlocks');
const REDRAW = 'codeBlocks.redraw';

function decorations(state: EditorState, code: Lowlight | undefined, options: CodeDrawing, onDiagram: (text: string) => void): DecorationSet {
  const found: Decoration[] = [];
  state.doc.descendants((node: PmNode, position: number) => {
    if (node.type.name !== 'codeBlock') return true;
    const language = String(node.attrs['language'] ?? '').toLowerCase();
    const text = node.textContent;
    if (language === 'mermaid') {
      if (text.trim() === '' || text.length > MAX_DIAGRAM) return false;
      const diagram = diagrams.get(text);
      if (!diagram) onDiagram(text);
      found.push(Decoration.widget(position + node.nodeSize, () => diagramElement(diagram, options), { side: -1, key: `diagram:${text.length}:${hash(text)}:${diagram ? ('src' in diagram ? 'ok' : 'error') : 'wait'}` }));
      return false;
    }
    if (code) {
      for (const run of highlight(code, language, text)) {
        found.push(Decoration.inline(position + 1 + run.from, position + 1 + run.to, { class: run.classes }));
      }
    }
    return false;
  });
  return DecorationSet.create(state.doc, found);
}

function diagramElement(diagram: Diagram | undefined, options: CodeDrawing): HTMLElement {
  const figure = document.createElement('figure');
  figure.className = 'page-diagram';
  figure.setAttribute('contenteditable', 'false');
  if (diagram && 'src' in diagram) {
    const image = document.createElement('img');
    image.className = 'page-diagram__picture';
    image.src = diagram.src;
    image.alt = options.words?.('diagram') ?? '';
    figure.append(image);
  } else if (diagram) {
    figure.classList.add('page-diagram--error');
    figure.textContent = options.words?.('diagram_error') ?? '';
  }
  return figure;
}

function hash(text: string): number {
  let h = 0;
  for (let at = 0; at < text.length; at++) h = (Math.imul(31, h) + text.charCodeAt(at)) | 0;
  return h >>> 0;
}

function hasBlock(state: EditorState, test: (language: string) => boolean): boolean {
  let found = false;
  state.doc.descendants((node: PmNode) => {
    if (found) return false;
    if (node.type.name === 'codeBlock') {
      if (test(String(node.attrs['language'] ?? '').toLowerCase())) found = true;
      return false;
    }
    return true;
  });
  return found;
}

/** Diagrams wait this long after the last keystroke, so typing one does not draw every letter. */
const DIAGRAM_PAUSE_MS = 400;
/** The pictures kept, the newest; an older text is drawn again when it comes back. */
const KEEP_DIAGRAMS = 64;

function keep(text: string, diagram: Diagram) {
  diagrams.delete(text);
  diagrams.set(text, diagram);
  while (diagrams.size > KEEP_DIAGRAMS) diagrams.delete(diagrams.keys().next().value as string);
}

/** The code blocks' colours and diagrams. */
export const CodeBlocks = Extension.create<CodeDrawing>({
  name: 'codeBlocks',
  addOptions: () => ({}),
  addProseMirrorPlugins() {
    const options = this.options;
    let code: Lowlight | undefined;
    let editor: EditorView | undefined;
    let pause: ReturnType<typeof setTimeout> | undefined;
    const wanted = new Set<string>();
    const redraw = () => {
      if (editor && !editor.isDestroyed) editor.dispatch(editor.state.tr.setMeta(REDRAW, true).setMeta('addToHistory', false));
    };
    const drawWanted = async () => {
      pause = undefined;
      const texts = [...wanted];
      wanted.clear();
      try {
        const engine = await loadMermaid(options.diagramColours?.());
        for (const text of texts) {
          try {
            const { svg } = await engine.render(`page-diagram-${++sequence}`, text);
            keep(text, { src: diagramSource(svg, options.diagramColours?.().paper) });
          } catch {
            keep(text, { error: true });
          }
        }
      } catch {
        for (const text of texts) keep(text, { error: true });
      }
      redraw();
    };
    const want = (text: string) => {
      wanted.add(text);
      if (pause) clearTimeout(pause);
      pause = setTimeout(() => void drawWanted(), DIAGRAM_PAUSE_MS);
    };
    return [new Plugin<DecorationSet>({
      key,
      view: (view) => {
        editor = view;
        const wantCode = () => {
          if (code || !hasBlock(view.state, (language) => language !== '' && language !== 'mermaid')) return;
          void loadLowlight().then((loaded) => {
            code = loaded;
            redraw();
          }, () => undefined);
        };
        wantCode();
        return {
          update: wantCode,
          destroy: () => {
            editor = undefined;
            if (pause) clearTimeout(pause);
          },
        };
      },
      state: {
        init: (_config, state) => decorations(state, code, options, want),
        apply: (tr, old, _before, state) => {
          if (!tr.docChanged && !tr.getMeta(REDRAW)) return old;
          return decorations(state, code, options, want);
        },
      },
      props: { decorations: (state) => key.getState(state) },
    })];
  },
});
