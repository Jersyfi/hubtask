// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The store in the installed apps: SQLite encrypted at rest under a key in the platform's keystore
// (ADR-0033 §4, offline-sync.md §9 rule 6). The database and the key live in the shell
// (`apps/webapp/src-tauri/src/copy.rs`), which a page cannot reach but through its commands; this
// is the port's side of them, one command per method, so the engine sees a `Storage` like any
// other. The shell is handed in as a function, so this package depends on no shell's library.
//
// The shell keeps a record as the text it is given and never reads it. A record is JSON, with one
// addition: bytes - a file captured offline (`holdsFiles`) - travel as base64 under a marker and
// come back as the kind they went in as, because JSON has no bytes and IndexedDB's structured
// clone, which the other stores keep, does.
//
// `clear()` deletes the database and keeps its key, which is what the engine asks for on a reset,
// `sync.cursor_too_old` and a forgotten device; `forget()` takes the key too, which is sign-out's
// alone (§9.6).

import type { Storage } from '../ports.ts';

/** How the shell is called: Tauri's `invoke`, or a fake of it in a test. */
export type ShellCall = (command: string, args: Record<string, unknown>) => Promise<unknown>;

/** The marker a run of bytes is held under. A record the server wrote never carries it. */
const BYTES = '$hubtask.bytes';

/** What a run of bytes was before it was written down, so it comes back as the same kind. */
type BytesKind = 'uint8array' | 'arraybuffer' | 'blob';

interface HeldBytes {
  readonly [BYTES]: string;
  readonly kind: BytesKind;
  readonly type?: string;
}

/** Characters per `String.fromCharCode` call: well below every engine's argument limit. */
const CHUNK = 0x8000;

/**
 * The copy's name: the API's origin and the account, each escaped to letters, digits, `.` and
 * `-`, every other byte as `_` and two hex digits - so two origins never share a name, and the
 * shell can make a directory and a file of it without reading a path out of it.
 */
export function copyNameFor(origin: string, accountId: string): string {
  return `${escaped(origin)}/${escaped(accountId)}`;
}

function escaped(text: string): string {
  let out = '';
  for (const byte of new TextEncoder().encode(text)) {
    const char = String.fromCharCode(byte);
    out += /[A-Za-z0-9.-]/.test(char) ? char : `_${byte.toString(16).toUpperCase().padStart(2, '0')}`;
  }
  return out;
}

export class SqliteStorage implements Storage {
  readonly #name: string;
  readonly #call: ShellCall;

  constructor(name: string, call: ShellCall) {
    this.#name = name;
    this.#call = call;
  }

  async get<T>(collection: string, id: string): Promise<T | undefined> {
    const held = await this.#call('copy_get', { name: this.#name, collection, id });
    return typeof held === 'string' ? (read(held) as T) : undefined;
  }

  async put<T>(collection: string, id: string, value: T): Promise<void> {
    await this.#call('copy_put', { name: this.#name, collection, id, value: JSON.stringify(await written(value)) });
  }

  async delete(collection: string, id: string): Promise<void> {
    await this.#call('copy_delete', { name: this.#name, collection, id });
  }

  async all<T>(collection: string): Promise<readonly T[]> {
    const held = await this.#call('copy_all', { name: this.#name, collection });
    return (held as readonly string[]).map((text) => read(text) as T);
  }

  async collections(): Promise<readonly string[]> {
    return (await this.#call('copy_collections', { name: this.#name })) as readonly string[];
  }

  async clear(): Promise<void> {
    await this.#call('copy_clear', { name: this.#name });
  }

  /** The copy and its key: what sign-out removes, so nothing left on the device can be read. */
  async forget(): Promise<void> {
    await this.#call('copy_forget', { name: this.#name });
  }
}

/** The value with every run of bytes in it written down; a `Blob` has to be read first. */
async function written(value: unknown): Promise<unknown> {
  if (value instanceof Uint8Array) return held(value, 'uint8array');
  if (value instanceof ArrayBuffer) return held(new Uint8Array(value), 'arraybuffer');
  if (typeof Blob !== 'undefined' && value instanceof Blob) {
    return { ...held(new Uint8Array(await value.arrayBuffer()), 'blob'), type: value.type };
  }
  if (Array.isArray(value)) return Promise.all(value.map(written));
  if (value !== null && typeof value === 'object') {
    const entries = await Promise.all(Object.entries(value).map(async ([key, each]) => [key, await written(each)] as const));
    return Object.fromEntries(entries);
  }
  return value;
}

function held(bytes: Uint8Array, kind: BytesKind): HeldBytes {
  let binary = '';
  for (let at = 0; at < bytes.length; at += CHUNK) {
    binary += String.fromCharCode(...bytes.subarray(at, at + CHUNK));
  }
  return { [BYTES]: btoa(binary), kind };
}

function read(text: string): unknown {
  return JSON.parse(text, (_key, value: unknown) => {
    if (value === null || typeof value !== 'object' || !(BYTES in value)) return value;
    const { [BYTES]: base64, kind, type } = value as HeldBytes;
    const binary = atob(base64);
    const bytes = new Uint8Array(binary.length);
    for (let at = 0; at < binary.length; at += 1) bytes[at] = binary.charCodeAt(at);
    if (kind === 'arraybuffer') return bytes.buffer;
    if (kind === 'blob') return new Blob([bytes], type ? { type } : {});
    return bytes;
  });
}
