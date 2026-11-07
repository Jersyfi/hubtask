// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// One import per chunk, shared by everyone who asks: the route, the preload after sign-in and the
// precache. With two imports of a screen in flight, the Linux shell (WebKitGTK) drew the screen
// before its module had finished evaluating, and it broke on a template still undefined.

/** The loader's first import, handed to every later call; a failed one is asked again. */
export function once<T>(load: () => Promise<T>): () => Promise<T> {
  let pending: Promise<T> | undefined;
  return () => {
    pending ??= load().catch((error: unknown) => {
      pending = undefined;
      throw error;
    });
    return pending;
  };
}

/** Each loader of a table, shared as `once` shares one. */
export function onceEach<T extends Record<string, () => Promise<unknown>>>(table: T): T {
  return Object.fromEntries(Object.entries(table).map(([name, load]) => [name, once(load)])) as T;
}
