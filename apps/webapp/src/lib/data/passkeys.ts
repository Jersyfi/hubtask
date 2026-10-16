// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The browser's half of adding a passkey (identity.md §23), kept apart from the store so it can be
 * tested without a browser: the server speaks WebAuthn's JSON form, where every binary value is
 * base64url without padding, and `navigator.credentials.create()` speaks buffers.
 *
 * Nothing here judges a ceremony. The server's verifier does; this only carries bytes across.
 */

import type { Passkey, PasskeyList, PasskeyRegistration, PasskeyRegistrationOptions } from '@hubtask/sync-engine';

/** base64url without padding, as WebAuthn's JSON form writes every binary value. */
export function toBase64Url(bytes: ArrayBuffer | Uint8Array): string {
  const view = bytes instanceof Uint8Array ? bytes : new Uint8Array(bytes);
  let binary = '';
  for (const byte of view) binary += String.fromCharCode(byte);
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

/** The bytes of a base64url value, padded or not. */
export function fromBase64Url(text: string): Uint8Array<ArrayBuffer> {
  const standard = text.replace(/-/g, '+').replace(/_/g, '/');
  const padded = standard + '='.repeat((4 - (standard.length % 4)) % 4);
  const binary = atob(padded);
  const bytes = new Uint8Array(new ArrayBuffer(binary.length));
  for (let index = 0; index < binary.length; index++) bytes[index] = binary.charCodeAt(index);
  return bytes;
}

/** The server's options, as `navigator.credentials.create({ publicKey })` takes them. */
export function creationOptionsOf(options: PasskeyRegistrationOptions): PublicKeyCredentialCreationOptions {
  const given = options.public_key;
  return {
    rp: { id: given.rp.id, name: given.rp.name },
    user: { id: fromBase64Url(given.user.id), name: given.user.name, displayName: given.user.displayName },
    challenge: fromBase64Url(given.challenge),
    pubKeyCredParams: given.pubKeyCredParams.map((param) => ({ type: param.type, alg: param.alg })),
    timeout: given.timeout,
    excludeCredentials: given.excludeCredentials.map((held) => ({
      type: held.type,
      id: fromBase64Url(held.id),
      ...(held.transports ? { transports: held.transports as AuthenticatorTransport[] } : {}),
    })),
    authenticatorSelection: {
      residentKey: given.authenticatorSelection.residentKey,
      requireResidentKey: given.authenticatorSelection.requireResidentKey,
      userVerification: given.authenticatorSelection.userVerification,
    },
    attestation: given.attestation,
  };
}

/** The transports the server knows; a browser may report others, which are not sent. */
const TRANSPORTS = ['usb', 'nfc', 'ble', 'hybrid', 'internal', 'smart-card'] as const;
type Transport = (typeof TRANSPORTS)[number];

/** What the shape of a created credential holds that this module reads. */
export interface CreatedCredential {
  readonly rawId: ArrayBuffer;
  readonly response: {
    readonly clientDataJSON: ArrayBuffer;
    readonly attestationObject: ArrayBuffer;
    getTransports?: () => string[];
  };
}

/** The ceremony's answer and the person's name, as `POST /accounts/me/passkeys:complete` takes it. */
export function registrationOf(credential: CreatedCredential, name: string): PasskeyRegistration {
  const reported = typeof credential.response.getTransports === 'function' ? credential.response.getTransports() : [];
  const transports = [...new Set(reported)].filter((each): each is Transport =>
    (TRANSPORTS as readonly string[]).includes(each),
  );
  return {
    name: name.trim(),
    raw_id: toBase64Url(credential.rawId),
    client_data_json: toBase64Url(credential.response.clientDataJSON),
    attestation_object: toBase64Url(credential.response.attestationObject),
    ...(transports.length > 0 ? { transports } : {}),
  };
}

/** What the browser can do: create a public-key credential at all. */
export interface PasskeyBrowser {
  readonly PublicKeyCredential?: unknown;
  readonly navigator?: { readonly credentials?: { readonly create?: unknown } };
}

/** Whether this browser can add a passkey. Where it cannot, no button is drawn (UC-ID-22, P-05). */
export function canCreatePasskeys(browser: PasskeyBrowser): boolean {
  return typeof browser.PublicKeyCredential !== 'undefined' &&
    typeof browser.navigator?.credentials?.create === 'function';
}

/**
 * Whether this page may add one for the workspace: its host must be the relying party's, because a
 * browser refuses a ceremony for another host - and in multi mode a page can be served at the
 * installation's host rather than the workspace's own.
 */
export function isAtRelyingParty(rpId: string | null | undefined, hostname: string): boolean {
  return typeof rpId === 'string' && rpId !== '' && rpId.toLowerCase() === hostname.toLowerCase();
}

/** How a ceremony that did not finish ended, as far as the person is concerned. */
export type CeremonyEnd = 'cancelled' | 'held' | 'failed';

/**
 * Reads the browser's refusal. Cancelled is the person's own choice - closing the prompt, or the
 * time running out - and says nothing (P-11). Held is an authenticator that already holds one of the
 * account's credentials (the excluded list). Anything else failed.
 */
export function ceremonyEndOf(cause: unknown): CeremonyEnd {
  const name = typeof cause === 'object' && cause !== null && 'name' in cause ? String(cause.name) : '';
  if (name === 'NotAllowedError' || name === 'AbortError') return 'cancelled';
  if (name === 'InvalidStateError') return 'held';
  return 'failed';
}

/** The name the dialog suggests: the word and the next number, so ten added in a row stay apart. */
export function suggestedName(word: string, held: readonly { name: string }[]): string {
  const taken = new Set(held.map((each) => each.name));
  for (let number = held.length + 1; ; number++) {
    const candidate = `${word} ${number}`;
    if (!taken.has(candidate)) return candidate;
  }
}

/** The most characters a name holds, counted as the server counts them: code points, after trimming. */
export const NAME_LIMIT = 64;

/** Whether a name would be accepted: not blank, and at most `NAME_LIMIT` code points once trimmed. */
export function nameFits(name: string): boolean {
  const trimmed = name.trim();
  return trimmed !== '' && [...trimmed].length <= NAME_LIMIT;
}

/**
 * Whether a passkey was made for another address than the workspace's - moved since, or restored
 * from another workspace. It opens nothing here, counts as no factor, and can always be removed.
 */
export function isStale(passkey: Pick<Passkey, 'rp_id'>, list: Pick<PasskeyList, 'rp_id'>): boolean {
  return Boolean(passkey.rp_id) && passkey.rp_id !== list.rp_id;
}

/** What a passkey's row offers for removing it: the button, or the reason in its place. */
export type Removal = { readonly offered: true } | { readonly offered: false; readonly reason: string };

/**
 * The server's own answer (P-05): `can_remove` for the passkeys of the workspace's address, and a
 * stale one always - asked only for the proof, which `auth.step_up_no_method` withholds from both.
 */
export function removalOf(passkey: Pick<Passkey, 'rp_id'>, list: PasskeyList): Removal {
  const reason = list.cannot_remove_reason;
  if (list.can_remove) return { offered: true };
  if (isStale(passkey, list) && reason !== 'auth.step_up_no_method') return { offered: true };
  return { offered: false, reason: reason ?? 'auth.mfa_required_by_tenant' };
}
