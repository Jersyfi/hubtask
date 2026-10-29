// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A whiteboard's scene (M4): its objects in the live document, the projection a board saves, the
 * geometry the canvas draws and hits by, and the board as a picture. The application hands in the
 * live document's connection and the colours; this package knows no address and no screen.
 */

export { Doc } from 'yjs';
export { Board, DEFAULT_SIZE, MAX_ELEMENTS, ROUTES, SHAPES, elementId } from './scene.ts';
export type { Element, ElementKind, Head, Route, Shape } from './scene.ts';
export {
  MAX_ZOOM, MIN_ZOOM, bounds, box, center, connectorPath, culled, ends, fit, hit, inside, labelPoint, outline, snap, toBoard,
  visibleBox, withinFrame, zoomAt,
} from './geometry.ts';
export type { Box, Point, Viewport } from './geometry.ts';
export { lines, outlinePath, toSvg } from './svg.ts';
export type { CardWords, Colours, PictureOptions } from './svg.ts';
