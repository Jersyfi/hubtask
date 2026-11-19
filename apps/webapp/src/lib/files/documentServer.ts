// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The workspace's document server as the settings page edits it (UC-MED-06 check 1): the kinds
 * offered, the body a choice sends and what moved. The largest vendor's browser office is a
 * choice the contract keeps and this page does not offer (the concept's E-14: never required).
 */

import type { DocumentServer } from '@hubtask/sync-engine';

export const DOCUMENT_SERVER_KINDS = ['NONE', 'COLLABORA_ONLINE', 'ONLYOFFICE', 'EURO_OFFICE'] as const;
export type DocumentServerKind = DocumentServer['kind'];

const origin = (url: string): string => url.trim().replace(/\/+$/, '');

/** The body a choice sends: an address only beside a server. */
export function choiceOf(kind: DocumentServerKind, url: string): DocumentServer {
  return kind === 'NONE' ? { kind } : { kind, url: origin(url) };
}

/** Whether the connection can be tested: a server with an address. */
export function isTestable(kind: DocumentServerKind, url: string): boolean {
  return kind !== 'NONE' && origin(url) !== '';
}

/** How many of the two values moved from what is stored. */
export function movedFrom(stored: DocumentServer | undefined, kind: DocumentServerKind, url: string): number {
  const was = choiceOf(stored?.kind ?? 'NONE', stored?.url ?? '');
  const next = choiceOf(kind, url);
  return (was.kind !== next.kind ? 1 : 0) + ((was.url ?? '') !== (next.url ?? '') ? 1 : 0);
}
