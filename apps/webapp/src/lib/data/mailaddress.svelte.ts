// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Mail addresses for every place (ADR-0092): the jumble's, a collection's and an entry's, on the
 * installation's receiving domain.
 *
 * **An address is what a place is**, so it is read rather than made: the first read mints it. It
 * is not a secret shown once, like the intake token - a member copies it again whenever needed -
 * but it is a key, and *New address* is how a leaked one is retired.
 *
 * **Without a receiving domain there is none**: `available` is false, and the places say so to the
 * people who can change it instead of drawing an empty field.
 */

import { engine } from './engine.ts';

export type MailTargetKind = 'JUMBLE' | 'COLLECTION' | 'ITEM';
export type SenderPolicy = 'MEMBERS' | 'ANYONE';

/** A place an address leads to. The jumble has no identifier. */
export interface MailTarget {
  readonly kind: MailTargetKind;
  readonly id?: string;
}

/** What `GET /mail/addresses` answers. */
export interface MailAddress {
  readonly available: boolean;
  readonly address?: string;
  readonly sender_policy?: SenderPolicy;
  readonly opened_by?: string;
}

const ADDRESSES = '/mail/addresses';

/** The read's path for one place. */
export function addressPath(target: MailTarget): string {
  const query = new URLSearchParams({ target_kind: target.kind });
  if (target.id) query.set('target_id', target.id);
  return `${ADDRESSES}?${query.toString()}`;
}

function bodyOf(target: MailTarget): Record<string, string> {
  return { target_kind: target.kind, ...(target.id ? { target_id: target.id } : {}) };
}

class MailAddresses {
  #held = $state<Record<string, MailAddress | 'failed'>>({});

  /** What was read for the place: the address, `failed`, or `undefined` while it is being read. */
  of(target: MailTarget): MailAddress | 'failed' | undefined {
    return this.#held[addressPath(target)];
  }

  /** Reads the place's address, minting it on the first read. **From `untrack`.** */
  async load(target: MailTarget): Promise<MailAddress | undefined> {
    const path = addressPath(target);
    const read = await engine.refresh<MailAddress>({ path });
    if (read.status === 'ready') {
      this.#held = { ...this.#held, [path]: read.data };
      return read.data;
    }
    if (read.status === 'failed') this.#held = { ...this.#held, [path]: 'failed' };
    return undefined;
  }

  /** Retires the address and answers the new one; whatever was sent to the old one stops arriving. */
  async rotate(target: MailTarget): Promise<MailAddress> {
    const next = await engine.mutate<MailAddress>('POST', `${ADDRESSES}:rotate`, bodyOf(target), {
      idempotencyKey: crypto.randomUUID(),
    });
    this.#held = { ...this.#held, [addressPath(target)]: next };
    return next;
  }

  /** Sets who may write into the place by mail. */
  async setPolicy(target: MailTarget, policy: SenderPolicy): Promise<MailAddress> {
    const next = await engine.mutate<MailAddress>(
      'POST',
      `${ADDRESSES}:set-sender-policy`,
      { ...bodyOf(target), sender_policy: policy },
      { idempotencyKey: crypto.randomUUID() },
    );
    this.#held = { ...this.#held, [addressPath(target)]: next };
    return next;
  }
}

export const mailAddresses = new MailAddresses();

/**
 * Copies an address. A clipboard the browser refuses is not a failure worth a sentence: the
 * address is on screen and selectable, the fallback every copy control has.
 */
export async function copyAddress(address: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(address);
    return true;
  } catch {
    return false;
  }
}

/**
 * Reads a place's address and copies it, for a menu item: answers the sentence to say - copied,
 * or the address itself where the browser refused the clipboard, so it can still be typed off.
 */
export async function copyPlaceAddress(
  target: MailTarget,
  say: (code: string, params?: Record<string, string>) => string,
): Promise<string | undefined> {
  const read = await mailAddresses.load(target);
  if (!read?.available || !read.address) return undefined;
  return (await copyAddress(read.address))
    ? say('app.mail.copied_menu')
    : say('app.mail.copy_failed', { address: read.address });
}
