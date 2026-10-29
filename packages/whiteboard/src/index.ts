// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A whiteboard's scene (M4): its objects in the live document, the projection a board saves, the
 * geometry the canvas draws and hits by, and the board as a picture. The application hands in the
 * live document's connection and the colours; this package knows no address and no screen.
 */

export { Doc } from 'yjs';
export { Board, DEFAULT_SIZE, MAX_ELEMENTS, ROUTES, SHAPES, elementId } from './scene.ts';
export type { Element, ElementKind, Head, Pen, Route, Shape } from './scene.ts';
export { MAX_STROKE_POINTS, PEN_WIDTH, simplify, strokePath, strokePoints, strokesOf, touches } from './draw.ts';
export { align, fromMinimap, minimap } from './guides.ts';
export type { Alignment, Guide, Minimap } from './guides.ts';
export { CLIPBOARD_FORMAT, copy, paste, readCopied, withGroups } from './clipboard.ts';
export type { Copied } from './clipboard.ts';
export { TEMPLATES, TEMPLATE_WORDS, template } from './templates.ts';
export type { TemplateInput, TemplateName } from './templates.ts';
export { Facilitation, MAX_TIMER, MAX_VOTES_PER_PERSON } from './facilitation.ts';
export type { Timer, Voting } from './facilitation.ts';
export { pdfOfJpeg } from './pdf.ts';
export {
  MAX_ZOOM, MIN_ZOOM, bounds, box, center, connectorPath, culled, ends, fit, hit, inside, labelPoint, outline, snap, toBoard,
  visibleBox, withinFrame, zoomAt,
} from './geometry.ts';
export type { Box, Point, Viewport } from './geometry.ts';
export { linkHost, lines, outlinePath, toSvg } from './svg.ts';
export type { CardWords, Colours, PictureOptions } from './svg.ts';
