// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A page's block editor (M3): the editor, its live document's connection, the Markdown projection
 * and the comparison of two versions. The application hands in the socket and the drawing of
 * references; this package knows no address, no token and no screen.
 */

export { addressOf, fromMarkdown, parseAddress, parseInline, textOf, toMarkdown } from './markdown.ts';
export type { DocNode, ReferenceAttrs } from './markdown.ts';
export { CLOSE_ACCESS_ENDED, LiveDocument } from './live.ts';
export type { LiveDocumentOptions, LiveSocket, LiveStatus } from './live.ts';
export { compareBodies, MAX_COMPARED_LINES } from './compare.ts';
export type { CompareLine } from './compare.ts';
export { createPageEditor, insertReference, markdownOf, passageTops, revealBlock, seedFromMarkdown, setPassages } from './editor.ts';
export type { Editor, PageEditorOptions, ReferenceDrawing, SelectedBlock } from './editor.ts';
export { findPassage, selectedWords } from './passages.ts';
export type { Passage, PassageRange } from './passages.ts';
export { Doc } from 'yjs';
