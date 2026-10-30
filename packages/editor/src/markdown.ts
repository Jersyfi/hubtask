// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A page's Markdown projection, both ways: the editor's document as the body the server keeps
 * (search, versions, references and export read it - offline-sync.md §13), and a body as the first
 * content of a page whose live document is still empty.
 *
 * **The notes' grammar** (reference.Extract on the server): a link to `hubtask:<kind>/<id>` is a
 * mention, an image of one a card or - for a view, a dashboard or a whiteboard - a live view; an
 * image of `hubtask:media/<id>` is one of the page's pictures and files, drawn as itself. The
 * label is always written empty, as the notes write it: a title in the body would show a reader
 * of the page what the referenced object is called even where they may not see it.
 *
 * Pure and headless - it works on the editor's JSON, never on a DOM - so the projection has tests
 * without a browser. What it does not know (a table, an image from outside) is written as its text.
 */

/** The editor's document as JSON, the shape ProseMirror's `toJSON` answers. */
export interface DocNode {
  type: string;
  attrs?: Record<string, unknown>;
  content?: DocNode[];
  marks?: { type: string; attrs?: Record<string, unknown> }[];
  text?: string;
}

/** A reference's address, as the body writes it. */
export interface ReferenceAttrs {
  kind: string;
  id: string;
  anchor: string | null;
}

const REFERENCE = /^hubtask:([a-z]+)\/([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})(?:#([A-Za-z0-9_-]{1,64}))?$/u;

/** The address of a reference, or nothing for any other link. */
export function parseAddress(href: string): ReferenceAttrs | undefined {
  const match = REFERENCE.exec(href);
  if (!match) return undefined;
  return { kind: match[1] ?? '', id: (match[2] ?? '').toLowerCase(), anchor: match[3] ?? null };
}

/** `hubtask:<kind>/<id>`, with `#<anchor>` where there is one. */
export function addressOf(ref: ReferenceAttrs): string {
  return `hubtask:${ref.kind}/${ref.id}${ref.anchor ? `#${ref.anchor}` : ''}`;
}

// --- The document as Markdown ------------------------------------------------------------------

/** The body the server keeps for a document. Blocks are separated by one empty line. */
export function toMarkdown(doc: DocNode): string {
  return blocks(doc.content ?? [], '').trimEnd() + '\n';
}

function blocks(nodes: readonly DocNode[], indent: string): string {
  return nodes.map((node) => block(node, indent)).filter((text) => text !== '').join('\n\n');
}

function block(node: DocNode, indent: string): string {
  switch (node.type) {
    case 'paragraph':
      return indent + inline(node.content ?? []).replace(/\n/gu, `\n${indent}`);
    case 'heading': {
      const level = Math.min(6, Math.max(1, Number(node.attrs?.['level'] ?? 1)));
      return `${indent}${'#'.repeat(level)} ${inline(node.content ?? [])}`;
    }
    case 'bulletList':
    case 'orderedList':
    case 'taskList':
      return list(node, indent);
    case 'blockquote':
      return blocks(node.content ?? [], '')
        .split('\n')
        .map((line) => `${indent}>${line === '' ? '' : ` ${line}`}`)
        .join('\n');
    case 'codeBlock': {
      const language = typeof node.attrs?.['language'] === 'string' ? node.attrs['language'] : '';
      const text = (node.content ?? []).map((each) => each.text ?? '').join('');
      const fence = text.includes('```') ? '~~~' : '```';
      return [`${indent}${fence}${language}`, ...text.split('\n').map((line) => indent + line), `${indent}${fence}`].join('\n');
    }
    case 'horizontalRule':
      return `${indent}---`;
    case 'referenceBlock':
      return `${indent}![](${addressOf(referenceOf(node))})`;
    default:
      return node.content ? blocks(node.content, indent) : '';
  }
}

function list(node: DocNode, indent: string): string {
  const ordered = node.type === 'orderedList';
  let number = Number(node.attrs?.['start'] ?? 1);
  return (node.content ?? [])
    .map((item) => {
      let marker = ordered ? `${number++}. ` : '- ';
      if (item.type === 'taskItem') marker += item.attrs?.['checked'] ? '[x] ' : '[ ] ';
      const inner = itemBlocks(item.content ?? [], indent + ' '.repeat(marker.length));
      // The first line carries the marker in place of its indent.
      return indent + marker + inner.slice(indent.length + marker.length);
    })
    .join('\n');
}

/** An item's blocks: a nested list follows its line directly, so the list stays tight. */
function itemBlocks(nodes: readonly DocNode[], indent: string): string {
  let out = '';
  nodes.forEach((node, at) => {
    const text = block(node, indent);
    if (text === '') return;
    if (at > 0) out += /List$/u.test(node.type) ? '\n' : '\n\n';
    out += text;
  });
  return out;
}

function inline(nodes: readonly DocNode[]): string {
  let out = '';
  for (const node of nodes) {
    if (node.type === 'hardBreak') {
      out += '\\\n';
      continue;
    }
    if (node.type === 'reference') {
      out += `[](${addressOf(referenceOf(node))})`;
      continue;
    }
    if (node.type !== 'text') continue;
    const marks = node.marks ?? [];
    const isCode = marks.some((mark) => mark.type === 'code');
    let text = isCode ? codeSpan(node.text ?? '') : escape(node.text ?? '');
    for (const mark of marks) {
      if (mark.type === 'bold') text = `**${text}**`;
      else if (mark.type === 'italic') text = `*${text}*`;
      else if (mark.type === 'strike') text = `~~${text}~~`;
      else if (mark.type === 'link' && typeof mark.attrs?.['href'] === 'string') text = `[${text}](${mark.attrs['href']})`;
    }
    out += text;
  }
  return out;
}

function codeSpan(text: string): string {
  const fence = text.includes('`') ? '``' : '`';
  return `${fence}${text}${fence}`;
}

/** Escapes what would otherwise read as Markdown; a line that starts a block is escaped too. */
function escape(text: string): string {
  return text
    .replace(/([\\`*_~[\]])/gu, '\\$1')
    .replace(/^(#{1,6} |[-+] |\d+\. |> )/u, '\\$1');
}

function referenceOf(node: DocNode): ReferenceAttrs {
  const attrs = node.attrs ?? {};
  return {
    kind: String(attrs['kind'] ?? 'entry'),
    id: String(attrs['id'] ?? ''),
    anchor: typeof attrs['anchor'] === 'string' && attrs['anchor'] !== '' ? attrs['anchor'] : null,
  };
}

// --- Markdown as a document ----------------------------------------------------------------------

/**
 * A body as the editor's first document. The grammar is the one `toMarkdown` writes, read
 * forgivingly: anything it does not know is a paragraph of its text.
 */
export function fromMarkdown(markdown: string): DocNode {
  const content = parseBlocks(markdown.replace(/\r\n?/gu, '\n').split('\n'));
  return { type: 'doc', content: content.length > 0 ? content : [{ type: 'paragraph' }] };
}

const HEADING = /^(#{1,6})\s+(.*)$/u;
const BULLET = /^([-*+])\s+(.*)$/u;
const ORDERED = /^(\d{1,9})[.)]\s+(.*)$/u;
const TASK = /^\[([ xX])\]\s+(.*)$/u;
const FENCE = /^(```|~~~)\s*([\w+-]*)\s*$/u;
const RULE = /^(?:-{3,}|\*{3,}|_{3,})\s*$/u;
const BLOCK_REFERENCE = /^!\[[^\]]*\]\((hubtask:[^)\s]+)\)\s*$/u;

function parseBlocks(lines: readonly string[]): DocNode[] {
  const out: DocNode[] = [];
  let i = 0;
  while (i < lines.length) {
    const line = lines[i] ?? '';
    if (line.trim() === '') {
      i++;
      continue;
    }
    const fence = FENCE.exec(line);
    if (fence) {
      const body: string[] = [];
      i++;
      while (i < lines.length && (lines[i] ?? '').trim() !== fence[1]) body.push(lines[i++] ?? '');
      i++;
      const text = body.join('\n');
      out.push({
        type: 'codeBlock',
        attrs: { language: fence[2] || null },
        ...(text === '' ? {} : { content: [{ type: 'text', text }] }),
      });
      continue;
    }
    const heading = HEADING.exec(line);
    if (heading) {
      out.push({ type: 'heading', attrs: { level: heading[1]?.length ?? 1 }, content: parseInline(heading[2] ?? '') });
      i++;
      continue;
    }
    if (RULE.test(line)) {
      out.push({ type: 'horizontalRule' });
      i++;
      continue;
    }
    const blockReference = BLOCK_REFERENCE.exec(line);
    const address = blockReference ? parseAddress(blockReference[1] ?? '') : undefined;
    if (address) {
      out.push({ type: 'referenceBlock', attrs: { ...address } });
      i++;
      continue;
    }
    if (line.startsWith('>')) {
      const quoted: string[] = [];
      while (i < lines.length && (lines[i] ?? '').startsWith('>')) quoted.push((lines[i++] ?? '').replace(/^> ?/u, ''));
      out.push({ type: 'blockquote', content: parseBlocks(quoted) });
      continue;
    }
    if (BULLET.test(line) || ORDERED.test(line)) {
      i = parseList(lines, i, out);
      continue;
    }
    const paragraph: string[] = [];
    while (i < lines.length) {
      const next = lines[i] ?? '';
      if (next.trim() === '' || (paragraph.length > 0 && startsBlock(next))) break;
      paragraph.push(next);
      i++;
    }
    out.push({ type: 'paragraph', content: parseInline(paragraph.join('\n')) });
  }
  return out;
}

function startsBlock(line: string): boolean {
  return HEADING.test(line) || FENCE.test(line) || RULE.test(line) || line.startsWith('>') || BULLET.test(line) || ORDERED.test(line);
}

/** Reads one list starting at `start`, with its nested lists, and answers the line after it. */
function parseList(lines: readonly string[], start: number, out: DocNode[]): number {
  const first = lines[start] ?? '';
  const ordered = ORDERED.exec(first);
  const items: DocNode[] = [];
  let isTask = false;
  let i = start;
  while (i < lines.length) {
    const line = lines[i] ?? '';
    const marker = ordered ? ORDERED.exec(line) : BULLET.exec(line);
    if (!marker) break;
    const markerLength = line.length - (marker[2] ?? '').length;
    let text = marker[2] ?? '';
    const task = ordered ? null : TASK.exec(text);
    if (task) {
      isTask = true;
      text = task[2] ?? '';
    }
    // The item's own lines: everything indented under the marker, and its continuation.
    const own: string[] = [text];
    i++;
    while (i < lines.length) {
      const next = lines[i] ?? '';
      if (next.trim() === '') {
        const after = lines[i + 1] ?? '';
        if (/^\s{2,}\S/u.test(after)) {
          own.push('');
          i++;
          continue;
        }
        break;
      }
      if (/^\s{2,}/u.test(next)) {
        own.push(next.replace(new RegExp(`^\\s{1,${markerLength}}`, 'u'), ''));
        i++;
        continue;
      }
      break;
    }
    const content = parseBlocks(own);
    items.push({
      type: task ? 'taskItem' : 'listItem',
      ...(task ? { attrs: { checked: task[1] !== ' ' } } : {}),
      content: content.length > 0 ? content : [{ type: 'paragraph' }],
    });
    // A blank line between items keeps the list; anything else ends it.
    if ((lines[i] ?? '').trim() === '' && (ordered ? ORDERED : BULLET).test(lines[i + 1] ?? '')) i++;
  }
  if (isTask) {
    for (const item of items) {
      if (item.type === 'listItem') Object.assign(item, { type: 'taskItem', attrs: { checked: false } });
    }
  }
  out.push(
    ordered
      ? { type: 'orderedList', attrs: { start: Number(ordered[1] ?? 1) }, content: items }
      : { type: isTask ? 'taskList' : 'bulletList', content: items },
  );
  return i;
}

/** The inline grammar: emphasis, code, strike, links, mentions and hard breaks. */
export function parseInline(text: string): DocNode[] {
  const out: DocNode[] = [];
  read(text, [], out);
  return merge(out);
}

type Mark = { type: string; attrs?: Record<string, unknown> };

function read(text: string, marks: readonly Mark[], out: DocNode[]): void {
  let buffer = '';
  const flush = () => {
    if (buffer !== '') out.push({ type: 'text', text: buffer, ...(marks.length > 0 ? { marks: [...marks] } : {}) });
    buffer = '';
  };
  let i = 0;
  while (i < text.length) {
    const rest = text.slice(i);
    const ch = text[i] ?? '';
    if (ch === '\\' && rest[1] === '\n') {
      flush();
      out.push({ type: 'hardBreak' });
      i += 2;
      continue;
    }
    if (ch === '\\' && rest.length > 1) {
      buffer += rest[1];
      i += 2;
      continue;
    }
    if (ch === '\n') {
      if (buffer.endsWith('  ')) {
        buffer = buffer.slice(0, -2);
        flush();
        out.push({ type: 'hardBreak' });
      } else buffer += ' ';
      i++;
      continue;
    }
    if (ch === '`') {
      const fence = rest.startsWith('``') ? '``' : '`';
      const end = text.indexOf(fence, i + fence.length);
      if (end > i) {
        flush();
        out.push({ type: 'text', text: text.slice(i + fence.length, end), marks: [...marks, { type: 'code' }] });
        i = end + fence.length;
        continue;
      }
    }
    const link = /^(!?)\[((?:\\.|[^\]\\])*)\]\(([^)\s]+)\)/u.exec(rest);
    if (link) {
      const address = parseAddress(link[3] ?? '');
      flush();
      if (address) {
        out.push({ type: 'reference', attrs: { ...address } });
      } else if (link[1] === '') {
        read(link[2] ?? '', [...marks, { type: 'link', attrs: { href: link[3] } }], out);
      } else {
        buffer += link[2] ?? '';
        flush();
      }
      i += link[0].length;
      continue;
    }
    const emphasis = /^(\*\*|__|~~|\*|_)/u.exec(rest);
    if (emphasis) {
      const delimiter = emphasis[1] ?? '';
      const end = closing(text, i + delimiter.length, delimiter);
      if (end > i + delimiter.length) {
        flush();
        const type = delimiter === '~~' ? 'strike' : delimiter.length === 2 ? 'bold' : 'italic';
        read(text.slice(i + delimiter.length, end), [...marks, { type }], out);
        i = end + delimiter.length;
        continue;
      }
    }
    buffer += ch;
    i++;
  }
  flush();
}

/** Where a delimiter closes, skipping escapes and code spans; -1 when it does not. */
function closing(text: string, from: number, delimiter: string): number {
  for (let i = from; i < text.length; i++) {
    const ch = text[i];
    if (ch === '\\') {
      i++;
      continue;
    }
    if (ch === '`') {
      const end = text.indexOf('`', i + 1);
      if (end > 0) i = end;
      continue;
    }
    if (text.startsWith(delimiter, i)) {
      // `**` inside a single `*` run is bold, not the close of the italic.
      if (delimiter.length === 1 && text[i + 1] === delimiter) {
        i++;
        continue;
      }
      return i;
    }
  }
  return -1;
}

/** Joins neighbouring text nodes with the same marks, so the document is the editor's own shape. */
function merge(nodes: DocNode[]): DocNode[] {
  const out: DocNode[] = [];
  for (const node of nodes) {
    const last = out[out.length - 1];
    if (last && last.type === 'text' && node.type === 'text' && JSON.stringify(last.marks ?? []) === JSON.stringify(node.marks ?? [])) {
      last.text = (last.text ?? '') + (node.text ?? '');
    } else out.push(node);
  }
  return out;
}

/** The plain words of a block, for a comment's quote and a heading's slug. */
export function textOf(node: DocNode): string {
  if (node.type === 'text') return node.text ?? '';
  return (node.content ?? []).map(textOf).join(node.type === 'doc' ? '\n' : '');
}
