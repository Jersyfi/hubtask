// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Whether the frame draws the emergency-access notice (ADR-0073 §4, UC-ID-16) for this route.
 *
 * Only on the start view: every member passes it, so nobody reads along unnoticed, while a work
 * view (whiteboard, entry, page, board) keeps its layout. The affected hub says it itself.
 */
export function isFrameEmergencyNoticeShown(routeName: string | null): boolean {
  return routeName === 'home';
}
