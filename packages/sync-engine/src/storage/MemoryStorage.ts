// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The store in memory: what the tests and the conformance runner use (F6-08), and what a runtime
// with no durable storage falls back to. It keeps exactly the port's promises and nothing more,
// which is what makes it the reference the shared storage test holds every other store to.

import type { Storage } from '../ports.ts';

export class MemoryStorage implements Storage {
  #collections = new Map<string, Map<string, unknown>>();

  async get<T>(collection: string, id: string): Promise<T | undefined> {
    const held = this.#collections.get(collection)?.get(id);
    return held === undefined ? undefined : (structuredClone(held) as T);
  }

  async put<T>(collection: string, id: string, value: T): Promise<void> {
    let held = this.#collections.get(collection);
    if (!held) {
      held = new Map();
      this.#collections.set(collection, held);
    }
    // A copy, so that a caller mutating what it put does not mutate what is held - the durable
    // stores hand back what was serialised, and this one should behave the same.
    held.set(id, structuredClone(value));
  }

  async delete(collection: string, id: string): Promise<void> {
    this.#collections.get(collection)?.delete(id);
  }

  async all<T>(collection: string): Promise<readonly T[]> {
    return [...(this.#collections.get(collection)?.values() ?? [])].map((v) => structuredClone(v)) as T[];
  }

  async collections(): Promise<readonly string[]> {
    return [...this.#collections.keys()].filter((name) => (this.#collections.get(name)?.size ?? 0) > 0);
  }

  async clear(): Promise<void> {
    this.#collections = new Map();
  }
}
