// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The server a shell speaks to. In the browser the API is the page's own origin (ADR-0028); a
 * shell is served from inside itself, so its API is the server the person chose (ADR-0031, the
 * address its sign-in asks for, ADR-0096). Before the first sign-in a build names one, and a
 * development build speaks to the local one.
 */

/** The server `make run` starts; what a development build of a shell speaks to. */
export const LOCAL_SERVER = 'http://localhost:8080';

/** The API's path below a server's origin, the same for every target (api-guidelines.md). */
export const API_PATH = '/api/v1';

/**
 * The API's base for a server: its origin and the API's path. Only an `https:` origin - or `http:`
 * on this machine, for development - is a server: a page served from inside the shell must never
 * send a credential in the clear across a network. Anything else answers `undefined`.
 */
export function apiBaseOf(server: string): string | undefined {
  let url: URL;
  try {
    url = new URL(server.trim());
  } catch {
    return undefined;
  }
  const local = url.hostname === 'localhost' || url.hostname === '127.0.0.1' || url.hostname === '[::1]';
  if (url.protocol !== 'https:' && !(url.protocol === 'http:' && local)) return undefined;
  if (url.username !== '' || url.password !== '') return undefined;
  return `${url.origin}${API_PATH}`;
}
