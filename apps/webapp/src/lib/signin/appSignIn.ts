// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The installed apps' sign-in through the system browser (ADR-0096, identity.md §15.3), the parts
 * that are only reasoning: which server an address names, whether it can be signed in to from this
 * app, and the authorization the browser opens. The screen and the shell do the rest.
 */

/** A workspace's address as the app speaks to it: the origin, and the host the card names. */
export interface WorkspaceAddress {
  readonly origin: string;
  readonly host: string;
}

const LOCAL = new Set(['localhost', '127.0.0.1', '[::1]']);

/**
 * The address a person typed, as a server: a host, or a full URL of which only the host is kept
 * (the approved preview's choice). HTTPS always - plain HTTP only on this machine, for development,
 * because the app sends a credential to it.
 */
export function addressOf(typed: string): WorkspaceAddress | undefined {
  const text = typed.trim();
  if (text === '' || /\s/.test(text)) return undefined;
  const hasScheme = /^[a-z][a-z0-9+.-]*:\/\//i.test(text);
  let url: URL;
  try {
    url = new URL(hasScheme ? text : `https://${text}`);
  } catch {
    return undefined;
  }
  if (url.username !== '' || url.password !== '' || url.hostname === '') return undefined;
  if (url.protocol !== 'https:' && url.protocol !== 'http:') return undefined;
  const local = LOCAL.has(url.hostname);
  if (url.protocol === 'http:' && !local) return undefined;
  if (!local && !url.hostname.includes('.')) return undefined;
  const scheme = local && (url.protocol === 'http:' || !hasScheme) ? 'http:' : 'https:';
  return { origin: `${scheme}//${url.host}`, host: url.host };
}

/** What the server answered about itself, narrowed to what the app decides by. */
export interface ServerSelf {
  readonly product_version?: string;
  readonly tenancy_mode?: string;
  readonly installation_url?: string | null;
  readonly app_client_id?: string;
}

/** Why an address cannot be signed in to, each with its sentence at the field. */
export type Refusal =
  | { readonly kind: 'unreachable' }
  | { readonly kind: 'not-hubtask' }
  | { readonly kind: 'older'; readonly server: string; readonly app: string }
  | { readonly kind: 'installation' };

/** The client the app signs in as, or why it cannot. */
export type Verdict = { readonly client: string } | { readonly refused: Refusal };

/**
 * Judges what an address answered (Decision 12): a Hubtask server of this app's version or newer,
 * at a workspace's address rather than an installation's own. `undefined` is an address where
 * nothing answered; `null` one that answered but not with a Hubtask manifest.
 */
export function judge(address: WorkspaceAddress, self: ServerSelf | null | undefined, appVersion: string): Verdict {
  if (self === undefined) return { refused: { kind: 'unreachable' } };
  if (self === null || typeof self.product_version !== 'string' || self.product_version === '') {
    return { refused: { kind: 'not-hubtask' } };
  }
  if (isOlder(self.product_version, appVersion) || typeof self.app_client_id !== 'string') {
    return { refused: { kind: 'older', server: self.product_version, app: appVersion } };
  }
  if (self.tenancy_mode === 'multi' && typeof self.installation_url === 'string') {
    try {
      if (new URL(self.installation_url).host === address.host) return { refused: { kind: 'installation' } };
    } catch {
      // An installation address that does not parse names nothing to compare with.
    }
  }
  return { client: self.app_client_id };
}

/**
 * Whether `server` is older than `app`, by their numeric release parts. A development build of the
 * app (`0.0.0`) is older than every server, so it never refuses one.
 */
export function isOlder(server: string, app: string): boolean {
  const parts = (version: string) => (version.split(/[-+]/)[0] ?? '').split('.').map((part) => Number.parseInt(part, 10) || 0);
  const [s, a] = [parts(server), parts(app)];
  for (let index = 0; index < Math.max(s.length, a.length); index++) {
    const difference = (s[index] ?? 0) - (a[index] ?? 0);
    if (difference !== 0) return difference < 0;
  }
  return false;
}

/** RFC 7636's verifier and its S256 challenge, from the page's own random source. */
export interface Pkce {
  readonly verifier: string;
  readonly challenge: string;
}

function base64Url(bytes: Uint8Array): string {
  let binary = '';
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary).replaceAll('+', '-').replaceAll('/', '_').replace(/=+$/, '');
}

/** The page's own random source, or a test's. */
export type Random = (bytes: Uint8Array<ArrayBuffer>) => Uint8Array<ArrayBuffer>;
const pageRandom: Random = (bytes) => crypto.getRandomValues(bytes);

export async function newPkce(random: Random = pageRandom): Promise<Pkce> {
  const verifier = base64Url(random(new Uint8Array(32)));
  const digest = new Uint8Array(await crypto.subtle.digest('SHA-256', new TextEncoder().encode(verifier)));
  return { verifier, challenge: base64Url(digest) };
}

export function newState(random: Random = pageRandom): string {
  return base64Url(random(new Uint8Array(16)));
}

/** The desktop listener's redirect for a port (identity.md §15.3). */
export function loopbackRedirect(port: number): string {
  return `http://127.0.0.1:${port}/oauth`;
}

/** What the consent in the browser is asked for. */
export interface Authorization {
  readonly client: string;
  readonly redirect: string;
  readonly challenge: string;
  readonly state: string;
  readonly device: string;
  readonly platform: string;
}

/**
 * The workspace's consent page, at its own host, in the system's browser: the card there offers
 * every way in the workspace allows before it (UC-SYN-07 check 3). No credential is in it - the
 * challenge proves nothing without the verifier this page keeps.
 */
export function consentUrl(address: WorkspaceAddress, request: Authorization): string {
  const url = new URL('/oauth/consent', address.origin);
  url.searchParams.set('client_id', request.client);
  url.searchParams.set('redirect_uri', request.redirect);
  url.searchParams.set('code_challenge', request.challenge);
  url.searchParams.set('code_challenge_method', 'S256');
  url.searchParams.set('state', request.state);
  url.searchParams.set('device', request.device);
  url.searchParams.set('platform', request.platform);
  return url.href;
}

/** The platforms the built-in client knows; the shell's system name is mapped onto them. */
export function appPlatformOf(system: string): string {
  return system === 'windows' || system === 'android' ? system : 'linux';
}
