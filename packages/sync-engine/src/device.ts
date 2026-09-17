// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The device: who this store is, to the server (offline-sync.md §3.1, §6).
//
// A device registers by turning up: the first pull or push under an identifier the account has
// not used writes its row, and the row keeps the platform, the name, the last cursor and the last
// contact. The identifier is a UUIDv7 the client minted - rule 1 of §9, the same rule an offline
// creation follows - held in the store beside the replica it identifies, and gone with the store
// at sign-out: a new sign-in is a new device, and the old row ages out of the server's list.
//
// Minted from the `Clock` port and Web Crypto rather than from `Date.now` and `Math.random`: the
// clock is what a test fixes, and the random bytes are the platform's own.

import type { Clock } from './ports.ts';

/** What the server is told about the device on every pull and push. */
export interface DeviceIdentity {
  /** A UUIDv7, minted here once. */
  readonly id: string;
  /** `web` for a browser; a shell names its platform. At most 40 characters. */
  readonly platform: string;
  /** How the device introduces itself in the list - the browser's name. At most 120 characters. */
  readonly displayName: string;
}

/** Random bytes from the platform, and nothing else: `Math.random` is not an identifier. */
function randomBytes(n: number): Uint8Array {
  const out = new Uint8Array(n);
  globalThis.crypto.getRandomValues(out);
  return out;
}

/**
 * A UUIDv7 at the clock's time: 48 bits of milliseconds, the version, 74 bits of randomness, the
 * variant - RFC 9562, the same layout the server mints (`core/domain/model/shared/ID.go`).
 */
export function mintUuidV7(clock: Clock): string {
  const now = Math.max(0, Math.trunc(clock.now()));
  const bytes = randomBytes(16);
  // 48 bits of time, most significant first.
  bytes[0] = (now / 2 ** 40) & 0xff;
  bytes[1] = (now / 2 ** 32) & 0xff;
  bytes[2] = (now / 2 ** 24) & 0xff;
  bytes[3] = (now / 2 ** 16) & 0xff;
  bytes[4] = (now / 2 ** 8) & 0xff;
  bytes[5] = now & 0xff;
  bytes[6] = ((bytes[6] ?? 0) & 0x0f) | 0x70;
  bytes[8] = ((bytes[8] ?? 0) & 0x3f) | 0x80;
  const hex = [...bytes].map((b) => b.toString(16).padStart(2, '0')).join('');
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}

/** The version nibble says 7: what N-04 accepts as a client-minted identifier. */
export function isUuidV7(value: string): boolean {
  return /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(value);
}

/** Bounded the way the contract bounds them, so a long browser name is cut rather than refused. */
export function boundedIdentity(id: string, platform: string, displayName: string): DeviceIdentity {
  return { id, platform: platform.slice(0, 40), displayName: displayName.slice(0, 120) };
}
