// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Client-side routing over the History API — a minimal in-house module, not a library. The
 * need is path matching, `pushState` and
 * link interception against a stable browser API, and a router dependency is a supply-chain
 * decision this small need does not justify.
 *
 * Real paths, never `#/`: ADR-0028's `index.html` fallback exists precisely so that a deep link
 * survives a reload, and hash routing would waste it. ADR-0030 fixes both points.
 *
 * Deliberately framework-free (no runes in here): the matching logic stays testable under
 * `node --test`, and a component binds the subscription to `$state` in one line.
 */

/**
 * Which area a route belongs to.
 *
 * Declared here rather than worked out later, because the mobile shell's one restriction is by
 * area: it ships the end-user features and profile configuration in full, and reaches
 * administration through the web app. A route that carried no area would be a route somebody has
 * to classify by reading it, and ADR-0032 asks for the tagging to exist
 * before the shell needs it.
 *
 * `end-user` is the default because it is the overwhelming majority, and a default that is wrong
 * for a new administrative screen is caught by the person adding one rather than by the shell.
 *
 * **Four, not ADR-0032's three.** [ADR-0070](../../../../docs/adr/ADR-0070-the-instance-layer.md)
 * §5 puts the level above the workspaces in this same web app — "a route area `/instance` in the
 * same web app, not a second bundle, not a second frame, not a second translation" — and the
 * shells exclude it exactly as they exclude administration. A fourth value rather than reusing
 * `administration`, because the two are different capabilities: an administrator runs a workspace
 * and an operator runs the installation, and a shell that shipped one because it shipped the other
 * would be shipping the control plane by accident.
 */
export type Area = 'end-user' | 'profile' | 'administration' | 'instance';

export interface Route {
  /** A stable name the shell switches on — never display text. */
  readonly name: string;
  /** A path pattern; `:segment` captures one path segment as a parameter. */
  readonly pattern: string;
  /** ADR-0032's area. Absent means `end-user`, which is what most screens are. */
  readonly area?: Area;
}

export interface Resolution {
  /** The name of the matched route, or `null` when nothing matched. */
  readonly name: string | null;
  readonly params: Readonly<Record<string, string>>;
  /** The path that was resolved, normalised, without query or fragment. */
  readonly path: string;
  /**
   * The query, as one value per name. A route's parameters are in the path; the query carries
   * what is open *beside* a screen - `?item=` on a collection is the entry in the detail pane
   * (ADR-0061 decision 4) - and is part of the address so that a reload and the back button keep
   * it. Never a search term: `security.md` §9's reason for `POST /search` stands.
   */
  readonly query: Readonly<Record<string, string>>;
  /** The area the matched route declared. `end-user` for a route that declared none. */
  readonly area: Area;
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
/** The query of an address, as one value per name; the fragment is not part of it. */
export function queryOf(input: string): Record<string, string> {
  const at = input.indexOf('?');
  if (at < 0) return {};
  const end = input.indexOf('#', at);
  const query: Record<string, string> = {};
  for (const [name, value] of new URLSearchParams(input.slice(at + 1, end < 0 ? undefined : end))) query[name] = value;
  return query;
}

export function resolve(routes: readonly Route[], input: string): Resolution {
  const path = normalisePath(input);
  const query = queryOf(input);
  for (const route of routes) {
    const params = matchPattern(route.pattern, path);
    if (params) return { name: route.name, params, path, query, area: route.area ?? 'end-user' };
  }
  return { name: null, params: {}, path, query, area: 'end-user' };
}

/** The address as the router compares it: the normalised path with its query, without a fragment. */
function addressOf(input: string): string {
  const path = normalisePath(input);
  const query = new URLSearchParams(queryOf(input)).toString();
  return query === '' ? path : `${path}?${query}`;
}

export type Unsubscribe = () => void;

/** How the last move came: a link (`push`), a redirect (`replace`), or back and forward (`pop`). */
export type Move = 'push' | 'replace' | 'pop';

/**
 * The stateful half: one instance per application, wired to the History API by `start()`.
 * Everything above this line is pure and carries the tests; everything below is thin enough
 * to be verified by reading it.
 */
/**
 * Whether a link stays in this application: the same scheme and host as the page. Not `origin`:
 * the URL standard gives a custom scheme's URL an opaque origin, `"null"`, while the page at
 * `tauri://localhost` (the shell on Linux and macOS) has a tuple one, so every link compared
 * unequal there and left as a full page load.
 */
export function isOwnAddress(url: URL, here: { readonly protocol: string; readonly host: string }): boolean {
  return url.protocol === here.protocol && url.host === here.host;
}

export class Router {
  readonly #routes: readonly Route[];
  readonly #listeners = new Set<(resolution: Resolution) => void>();
  #current: Resolution;
  readonly #fullLoad?: (path: string) => boolean;
  readonly #admits?: (path: string) => boolean;
  #move: Move = 'replace';
  #entry = '';
  #entries = 0;

  /**
   * `fullLoad` may take a navigation as a full page load instead - a newer release to load
   * (`frame/release.ts`) - and answers whether it did. `admits` may hold a move the reader has
   * not agreed to yet - a screen with unsaved changes (`frame/leave.ts`) - and answers whether it
   * goes; a held move is made later through `navigate`.
   */
  constructor(
    routes: readonly Route[],
    initialPath: string = window.location.pathname + window.location.search,
    options: { fullLoad?: (path: string) => boolean; admits?: (path: string) => boolean } = {},
  ) {
    this.#routes = routes;
    this.#current = resolve(routes, initialPath);
    this.#fullLoad = options.fullLoad;
    this.#admits = options.admits;
  }

  get current(): Resolution {
    return this.#current;
  }

  /** How the current address was reached. */
  get move(): Move {
    return this.#move;
  }

  /**
   * The session history entry the current address stands on, kept in `history.state`, so that
   * what belongs to one entry - its scroll position - is found again on *back*.
   */
  get entry(): string {
    return this.#entry;
  }

  subscribe(listener: (resolution: Resolution) => void): Unsubscribe {
    this.#listeners.add(listener);
    listener(this.#current);
    return () => this.#listeners.delete(listener);
  }

  /** Navigate to a path, entering it into the session history. */
  navigate(path: string): void {
    if (addressOf(path) === this.#address()) return;
    if (this.#admits && !this.#admits(path)) return;
    if (this.#fullLoad?.(path)) return;
    this.#entry = this.#newEntry();
    history.pushState({ entry: this.#entry }, '', path);
    this.#apply(path, 'push');
  }

  /**
   * Replaces the current entry rather than adding one: what a redirect does, so that the back
   * button goes to where the reader came from and not to the address that sent them on.
   */
  replace(path: string): void {
    if (addressOf(path) === this.#address()) return;
    history.replaceState({ entry: this.#entry }, '', path);
    this.#apply(path, 'replace');
  }

  #newEntry(): string {
    this.#entries += 1;
    return `${Date.now().toString(36)}-${this.#entries}`;
  }

  #address(): string {
    const query = new URLSearchParams(this.#current.query).toString();
    return query === '' ? this.#current.path : `${this.#current.path}?${query}`;
  }

  /**
   * Wire the instance to the document: back and forward buttons, and every plain left click on
   * a same-origin link. Anything else — modified clicks, downloads, `target`, foreign origins —
   * stays with the browser.
   */
  start(): Unsubscribe {
    const kept = (history.state as { entry?: unknown } | null)?.entry;
    this.#entry = typeof kept === 'string' ? kept : this.#newEntry();
    if (kept !== this.#entry) history.replaceState({ ...(history.state ?? {}), entry: this.#entry }, '');
    const onPopState = (event: PopStateEvent) => {
      const path = window.location.pathname + window.location.search;
      // The browser has already moved the address; a held move puts it back, so the address and
      // the screen agree while the reader decides.
      if (this.#admits && addressOf(path) !== this.#address() && !this.#admits(path)) {
        history.pushState({ entry: this.#entry }, '', this.#address());
        return;
      }
      const entry = (event.state as { entry?: unknown } | null)?.entry;
      this.#entry = typeof entry === 'string' ? entry : this.#newEntry();
      this.#apply(path, 'pop');
    };
    const onClick = (event: MouseEvent) => {
      if (event.defaultPrevented || event.button !== 0) return;
      if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
      const anchor = (event.target as Element | null)?.closest('a');
      if (!anchor || anchor.target || anchor.hasAttribute('download')) return;
      const url = new URL(anchor.href, window.location.href);
      if (!isOwnAddress(url, window.location)) return;
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

  #apply(path: string, move: Move): void {
    this.#move = move;
    this.#current = resolve(this.#routes, path);
    for (const listener of this.#listeners) listener(this.#current);
  }
}
