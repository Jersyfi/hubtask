// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The idempotency key of a write, per intent rather than per call (`api-guidelines.md`,
 * idempotency).
 *
 * A write whose answer was lost - a timeout, a dropped connection, a `5xx` - may have been carried
 * out. A fresh key on the person's second press makes it a second write: two tickets, two time
 * records. So the same method, path and content keep their key until the server has decided about
 * it; then the next press is a new intent. An answered refusal (`4xx`) releases the key too: the
 * server stores it, and keeping the key would replay the stored refusal after the cause was
 * resolved (a quota raised, a field fixed).
 *
 * Not for a route whose answer is shown once (`rest.ShownOnceRoutes`): its repeat answers the
 * status without the credential, so such a call keeps minting a key per call.
 */

/** How long an undecided intent is kept: the server replays a key for at least 24 hours. */
const KEPT_MS = 23 * 60 * 60 * 1000;

interface Open {
  readonly key: string;
  readonly since: number;
}

export class Intents {
  readonly #open = new Map<string, Open>();
  readonly #mint: () => string;
  readonly #now: () => number;

  constructor(mint: () => string = () => crypto.randomUUID(), now: () => number = () => Date.now()) {
    this.#mint = mint;
    this.#now = now;
  }

  /** The key for this write: the undecided one of the same content, or a new one. */
  keyFor(method: string, path: string, body: unknown): string {
    const content = contentOf(method, path, body);
    const open = this.#open.get(content);
    if (open && this.#now() - open.since < KEPT_MS) return open.key;
    const key = this.#mint();
    this.#open.set(content, { key, since: this.#now() });
    return key;
  }

  /** Records how the write ended: decided releases the key, undecided keeps it for the retry. */
  settle(method: string, path: string, body: unknown, failure?: unknown): void {
    if (failure !== undefined && !decided(failure)) return;
    this.#open.delete(contentOf(method, path, body));
  }
}

/** Whether the server answered with a decision: anything but a lost answer or a `5xx`. */
function decided(failure: unknown): boolean {
  const status = (failure as { status?: unknown } | null)?.status;
  return typeof status === 'number' && status >= 400 && status < 500;
}

function contentOf(method: string, path: string, body: unknown): string {
  return `${method} ${path} ${stable(body)}`;
}

/** JSON with the keys sorted, so the same content is the same text whatever order it was built in. */
function stable(value: unknown): string {
  if (value === undefined) return '';
  return JSON.stringify(value, (_key, inner: unknown) => {
    if (inner === null || typeof inner !== 'object' || Array.isArray(inner)) return inner;
    return Object.fromEntries(Object.entries(inner as Record<string, unknown>).sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0)));
  });
}
