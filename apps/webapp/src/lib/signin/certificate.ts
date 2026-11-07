// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Where a SAML provider's certificates stand (identity.md §26.1): ending within the server's first
 * warning of 30 days, ended, or neither. An ended certificate refuses every answer of the provider,
 * so nobody signs in through it; the sign-in settings say so before the e-mail is the only sign.
 */
export type CertificateState = 'ending' | 'ended';

/** The server's first warning, in days (`SAMLExpiryWarnings`). */
export const CERTIFICATE_WARNING_DAYS = 30;

const DAY = 24 * 60 * 60 * 1000;

export function certificateState(expiresAt: string | null | undefined, now: number): CertificateState | undefined {
  if (!expiresAt) return undefined;
  const at = Date.parse(expiresAt);
  if (Number.isNaN(at)) return undefined;
  if (at <= now) return 'ended';
  return at - now <= CERTIFICATE_WARNING_DAYS * DAY ? 'ending' : undefined;
}
