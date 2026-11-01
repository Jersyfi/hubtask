// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// What the application says when a screen breaks: a client reference and the details a person can
// copy into a support request. The reference names this occurrence; the details say which build,
// which address and which browser, and the server's request ID where the failure carried one, so
// support can find the server's line for it. Never content: no error message, no title, no field
// value - an error's text can quote what the person was looking at (rule 10's reason, on the
// client).

/** The facts of one failure, as the frame gathers them. */
export interface CrashFacts {
  readonly reference: string;
  /** The build's entry script, which tells one release from another. */
  readonly build: string | null;
  /** The address, without its query: a query can carry a search the person typed. */
  readonly path: string;
  readonly userAgent: string;
  readonly at: Date;
  readonly error: unknown;
}

/** A short reference for one failure, from the random source handed in. */
export function newReference(random: () => number = Math.random): string {
  const alphabet = '0123456789ABCDEFGHJKMNPQRSTVWXYZ';
  let out = 'C-';
  for (let i = 0; i < 8; i++) out += alphabet[Math.floor(random() * alphabet.length) % alphabet.length];
  return out;
}

/** The server's request ID a failure carries, where it carries one (a TransportError does). */
export function requestIdOf(error: unknown): string | undefined {
  if (typeof error !== 'object' || error === null || !('requestId' in error)) return undefined;
  const id = (error as { requestId?: unknown }).requestId;
  return typeof id === 'string' && id !== '' ? id : undefined;
}

/** The kind of failure, never its message. */
export function kindOf(error: unknown): string {
  if (error instanceof Error) return error.name || 'Error';
  return typeof error;
}

/** The text "Copy details" puts on the clipboard. */
export function crashDetails(facts: CrashFacts): string {
  const requestId = requestIdOf(facts.error);
  return [
    `reference: ${facts.reference}`,
    `build: ${facts.build ?? 'unknown'}`,
    `path: ${facts.path}`,
    `at: ${facts.at.toISOString()}`,
    `browser: ${facts.userAgent}`,
    `failure: ${kindOf(facts.error)}`,
    ...(requestId ? [`request_id: ${requestId}`] : []),
  ].join('\n');
}

/**
 * Whether a window-level failure is the browser's noise rather than the application's: a resize
 * observer that skipped a frame, or a request the application itself cancelled.
 */
export function isNoise(error: unknown, message = ''): boolean {
  if (/^ResizeObserver loop/.test(message)) return true;
  return error instanceof Error && error.name === 'AbortError';
}
