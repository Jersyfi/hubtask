// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A step-up with a passkey from an installed app (ADR-0096 decision 5, identity.md §15.3): the app
 * asks its server for the options as any step-up does, the system browser runs the ceremony at the
 * workspace's own host - the passkey's relying party - and hands the answer back to the app's
 * loopback address, from where the app sends it as its proof.
 *
 * **What travels.** The options go in the address's fragment, which no request carries; they hold
 * a challenge bound to the app's session, worth nothing to anybody else. The answer comes back as
 * the loopback redirect's `code`, like the sign-in's: bound to that challenge, single use, and
 * spent by the app's own session or by nobody. The loopback address is always `127.0.0.1` - only
 * the port comes from the address, so the page cannot be turned into a redirect to anywhere else.
 */

import type { PasskeyAssertionOptions } from '@hubtask/sync-engine';

import { fromBase64Url, toBase64Url, type KeyAnswer } from '../data/passkeys.ts';
import { loopbackRedirect } from './appSignIn.ts';

/** Where the workspace's page that asks the browser for the passkey lives. */
export const RELAY_PATH = '/app/passkey';

/** What the app hands the page in the fragment. */
export interface Relay {
  readonly port: number;
  readonly state: string;
  readonly options: PasskeyAssertionOptions;
}

function encoded(value: unknown): string {
  return toBase64Url(new TextEncoder().encode(JSON.stringify(value)));
}

function decoded(text: string): unknown {
  try {
    return JSON.parse(new TextDecoder().decode(fromBase64Url(text)));
  } catch {
    return undefined;
  }
}

/** The page the system browser opens, at the workspace's origin. */
export function relayUrl(origin: string, relay: Relay): string {
  const fragment = new URLSearchParams({ port: String(relay.port), state: relay.state, options: encoded(relay.options) });
  return `${new URL(RELAY_PATH, origin).href}#${fragment.toString()}`;
}

const STATE = /^[A-Za-z0-9_-]{8,128}$/;

/** The fragment read back, or `undefined` for anything that is not the app's request. */
export function readRelay(hash: string): Relay | undefined {
  const fields = new URLSearchParams(hash.replace(/^#/, ''));
  const port = Number(fields.get('port'));
  const state = fields.get('state') ?? '';
  const options = decoded(fields.get('options') ?? '') as PasskeyAssertionOptions | undefined;
  if (!Number.isInteger(port) || port < 1 || port > 65_535 || !STATE.test(state)) return undefined;
  const given = options?.public_key;
  if (typeof given?.challenge !== 'string' || typeof given.rpId !== 'string' || !Array.isArray(given.allowCredentials)) {
    return undefined;
  }
  return { port, state, options: options as PasskeyAssertionOptions };
}

/** Where the browser takes the answer: the app's loopback address and nothing else. */
export function answerUrl(relay: Pick<Relay, 'port' | 'state'>, answer: KeyAnswer): string {
  const url = new URL(loopbackRedirect(relay.port));
  url.searchParams.set('state', relay.state);
  url.searchParams.set('code', encoded(answer));
  return url.href;
}

/** Where the browser goes when the person gives up there: the app hears that nothing came. */
export function declinedUrl(relay: Pick<Relay, 'port' | 'state'>): string {
  const url = new URL(loopbackRedirect(relay.port));
  url.searchParams.set('state', relay.state);
  url.searchParams.set('error', 'access_denied');
  return url.href;
}

/** The answer as the app reads it from the redirect's `code`, or `undefined` for anything else. */
export function answerOf(code: string | undefined): KeyAnswer | undefined {
  if (code === undefined) return undefined;
  const value = decoded(code) as Partial<Record<keyof KeyAnswer, unknown>> | undefined;
  if (value === undefined || value === null || typeof value !== 'object') return undefined;
  const { raw_id, client_data_json, authenticator_data, signature } = value;
  if ([raw_id, client_data_json, authenticator_data, signature].some((part) => typeof part !== 'string' || part === '')) {
    return undefined;
  }
  return { raw_id, client_data_json, authenticator_data, signature } as KeyAnswer;
}
