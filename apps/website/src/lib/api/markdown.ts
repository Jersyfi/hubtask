// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The subset of Markdown the specification's descriptions use, read into a tree.
 *
 * Not a Markdown library: one is a dependency, and this site has counted every one it has. The
 * descriptions in `api/openapi.yaml` are written by hand in a small dialect - paragraphs, inline
 * code, emphasis, links, bulleted lists, the occasional fenced block - and this reads exactly
 * that. What it does not know it renders as text, never as markup: the output is a tree the page
 * renders element by element, so no string from the document ever becomes HTML by concatenation.
 */

export type Inline =
  | { readonly kind: 'text'; readonly text: string }
  | { readonly kind: 'code'; readonly text: string }
  | { readonly kind: 'strong'; readonly text: string }
  | { readonly kind: 'em'; readonly text: string }
  | { readonly kind: 'link'; readonly text: string; readonly href: string };

export type Block =
  | { readonly kind: 'paragraph'; readonly inlines: readonly Inline[] }
  | { readonly kind: 'list'; readonly items: readonly (readonly Inline[])[] }
  | { readonly kind: 'code'; readonly text: string; readonly language?: string };

const LIST_ITEM = /^\s*[*-]\s+(.*)$/;
const FENCE = /^```(\w*)\s*$/;

/** The inline dialect: `code`, **strong**, *em*, [text](href). Anything else is text. */
export function inlines(text: string): Inline[] {
  const out: Inline[] = [];
  const pattern = /(`[^`]+`)|(\*\*[^*]+\*\*)|(\*[^*\s][^*]*\*)|(\[[^\]]+\]\([^)\s]+\))/g;
  let last = 0;
  for (const match of text.matchAll(pattern)) {
    const index = match.index ?? 0;
    if (index > last) out.push({ kind: 'text', text: text.slice(last, index) });
    const token = match[0];
    if (token.startsWith('`')) out.push({ kind: 'code', text: token.slice(1, -1) });
    else if (token.startsWith('**')) out.push({ kind: 'strong', text: token.slice(2, -2) });
    else if (token.startsWith('[')) {
      const link = /^\[([^\]]+)\]\(([^)\s]+)\)$/.exec(token);
      if (link) out.push({ kind: 'link', text: link[1] ?? '', href: link[2] ?? '' });
    } else out.push({ kind: 'em', text: token.slice(1, -1) });
    last = index + token.length;
  }
  if (last < text.length) out.push({ kind: 'text', text: text.slice(last) });
  return out;
}

/** The block dialect: paragraphs separated by blank lines, bulleted lists, fenced code. */
export function blocks(source: string): Block[] {
  const out: Block[] = [];
  const lines = source.replace(/\r\n/g, '\n').split('\n');
  let paragraph: string[] = [];
  let list: string[] = [];
  const flushParagraph = () => {
    if (paragraph.length > 0) out.push({ kind: 'paragraph', inlines: inlines(paragraph.join(' ').trim()) });
    paragraph = [];
  };
  const flushList = () => {
    if (list.length > 0) out.push({ kind: 'list', items: list.map((item) => inlines(item.trim())) });
    list = [];
  };

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i] ?? '';
    const fence = FENCE.exec(line);
    if (fence) {
      flushParagraph();
      flushList();
      const body: string[] = [];
      i++;
      while (i < lines.length && !FENCE.test(lines[i] ?? '')) {
        body.push(lines[i] ?? '');
        i++;
      }
      out.push({ kind: 'code', text: body.join('\n'), language: fence[1] || undefined });
      continue;
    }
    const item = LIST_ITEM.exec(line);
    if (item) {
      flushParagraph();
      list.push(item[1] ?? '');
      continue;
    }
    if (line.trim() === '') {
      flushParagraph();
      flushList();
      continue;
    }
    // A continuation line of a list item is indented; anything else ends the list.
    if (list.length > 0 && /^\s+\S/.test(line)) {
      list[list.length - 1] += ' ' + line.trim();
      continue;
    }
    flushList();
    paragraph.push(line.trim());
  }
  flushParagraph();
  flushList();
  return out;
}

/** The first sentence, as a plain string, for a place that has room for one line. */
export function firstSentence(source: string): string {
  const first = blocks(source).find((block) => block.kind === 'paragraph');
  if (!first || first.kind !== 'paragraph') return '';
  const text = first.inlines.map((inline) => inline.text).join('');
  const end = text.search(/[.!?](\s|$)/);
  return end === -1 ? text : text.slice(0, end + 1);
}
