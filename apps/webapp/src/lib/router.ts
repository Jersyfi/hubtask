// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Client-side routing over the History API — a minimal in-house module, not a library. The
 * reasoning is recorded in the W-06 pull request: the need is path matching, `pushState` and
 * link interception against a stable browser API, and a router dependency is a supply-chain
 * decision this small need does not justify (CLAUDE.md).
 *
 * Real paths, never `#/`: ADR-0028's `index.html` fallback exists precisely so that a deep link
 * survives a reload, and hash routing would waste it. ADR-0030 fixes both points.
 *
 * Deliberately framework-free (no runes in here): the matching logic stays testable under
 * `node --test`, and a component binds the subscription to `$state` in one line.
 */

export interface Route {
  /** A stable name the shell switches on — never display text. */
  readonly name: string;
  /** A path pattern; `:segment` captures one path segment as a parameter. */
  readonly pattern: string;
}

export interface Resolution {
  /** The name of the matched route, or `null` when nothing matched. */
  readonly name: string | null;
  readonly params: Readonly<Record<string, string>>;
  /** The path that was resolved, normalised, without query or fragment. */
  readonly path: string;
}

/** Strip query and fragment, collapse a trailing slash: `/hubs/` and `/hubs` are one route. */
export function normalisePath(input: string): string {
  const path = input.split(/[?#]/, 1)[0] ?? '';
  if (path === '' || path === '/') return '/';
  return path.endsWith('/') ? path.slice(0, -1) : path;
}

/** Match one pattern against a normalised path; `null` when it does not fit. */
function matchPattern(pattern: string, path: string): Record<string, string> | null {
  const patternSegments = normalisePath(pattern).split('/');
  const pathSegments = path.split('/');
  if (patternSegments.length !== pathSegments.length) return null;

  const params: Record<string, string> = {};
  for (let i = 0; i < patternSegments.length; i++) {
    const expected = patternSegments[i] ?? '';
    const actual = pathSegments[i] ?? '';
    if (expected.startsWith(':')) {
      // An empty segment is no segment: `/hubs/` must not resolve as `/hubs/:id`.
      if (actual === '') return null;
      params[expected.slice(1)] = decodeURIComponent(actual);
    } else if (expected !== actual) {
      return null;
    }
  }
  return params;
}

/** Resolve a path against the table. First match wins, so order the table specific-first. */
export function resolve(routes: readonly Route[], input: string): Resolution {
  const path = normalisePath(input);
  for (const route of routes) {
    const params = matchPattern(route.pattern, path);
    if (params) return { name: route.name, params, path };
  }
  return { name: null, params: {}, path };
}

export type Unsubscribe = () => void;

/**
 * The stateful half: one instance per application, wired to the History API by `start()`.
 * Everything above this line is pure and carries the tests; everything below is thin enough
 * to be verified by reading it.
 */
export class Router {
  readonly #routes: readonly Route[];
  readonly #listeners = new Set<(resolution: Resolution) => void>();
  #current: Resolution;

  constructor(routes: readonly Route[], initialPath: string = window.location.pathname) {
    this.#routes = routes;
    this.#current = resolve(routes, initialPath);
  }

  get current(): Resolution {
    return this.#current;
  }

  subscribe(listener: (resolution: Resolution) => void): Unsubscribe {
    this.#listeners.add(listener);
    listener(this.#current);
    return () => this.#listeners.delete(listener);
  }

  /** Navigate to a path, entering it into the session history. */
  navigate(path: string): void {
    if (normalisePath(path) === this.#current.path) return;
    history.pushState(null, '', path);
    this.#apply(path);
  }

  /**
   * Wire the instance to the document: back and forward buttons, and every plain left click on
   * a same-origin link. Anything else — modified clicks, downloads, `target`, foreign origins —
   * stays with the browser.
   */
  start(): Unsubscribe {
    const onPopState = () => this.#apply(window.location.pathname);
    const onClick = (event: MouseEvent) => {
      if (event.defaultPrevented || event.button !== 0) return;
      if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
      const anchor = (event.target as Element | null)?.closest('a');
      if (!anchor || anchor.target || anchor.hasAttribute('download')) return;
      const url = new URL(anchor.href, window.location.href);
      if (url.origin !== window.location.origin) return;
      event.preventDefault();
      this.navigate(url.pathname + url.search);
    };
    window.addEventListener('popstate', onPopState);
    document.addEventListener('click', onClick);
    return () => {
      window.removeEventListener('popstate', onPopState);
      document.removeEventListener('click', onClick);
    };
  }

  #apply(path: string): void {
    this.#current = resolve(this.#routes, path);
    for (const listener of this.#listeners) listener(this.#current);
  }
}
