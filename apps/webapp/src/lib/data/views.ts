// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What this client decides about a saved view, its export and its feed. Pure, so each is tested.
 *
 * **A view is the one place this client's own words travel through the API.** The server "stores
 * the query, validates it against the same catalogue as `POST /items:query`, and interprets neither
 * the layout nor the visible fields — both are the client's vocabulary, echoed back exactly as
 * stored". So opening a view applies *both* halves: the query the server checked, and the layout
 * only a client knows what to do with. A client that applied the query alone would drop the half
 * the field was added for.
 *
 * **The stored query is a whole query document, scope included.** That is the contract's own
 * wording — "the query document of `POST /items:query`, stored as sent" — and it is not decoration:
 * `:export` *executes* the stored query on the server, and an unanchored one is refused with
 * `query.scope_required` before a single row is rendered. A view that stored only the question
 * would be a view nobody could export, which the walk against a running server is what showed.
 *
 * **Applying one on screen uses the question half alone.** The list already knows which collection
 * it is looking at, so opening a view there asks its filter and its order of *those* entries. The
 * stored scope is what the export runs against; the screen's scope is what the screen runs against,
 * and the two are different questions that happen to share a filter.
 *
 * **`PUBLIC_LINK` is declared and refused.** The switcher shows all three, because one that omitted
 * it would disagree with the contract, and offers two — the third carries the server's own reason.
 */

import type { ItemsQuery } from './items.svelte.ts';

/** The three the contract's enum declares. */
export const SHARING = ['PRIVATE', 'SCOPE', 'PUBLIC_LINK'] as const;

export type Sharing = (typeof SHARING)[number];

/**
 * Why a sharing choice cannot be made, or nothing where it can.
 *
 * Both reasons are the **server's own codes**, so one fact has one sentence whether this client saw
 * the refusal coming or the server sent it.
 */
export function sharingRefusal(sharing: Sharing, scopeType: string): string | undefined {
  if (sharing === 'PUBLIC_LINK') return 'views.public_link_not_available';
  if (sharing === 'SCOPE' && scopeType === 'ACCOUNT') return 'views.account_scope_not_shareable';
  return undefined;
}

/** The scopes a view can be saved at. `ACCOUNT` is private and needs no permission. */
export const VIEW_SCOPES = ['ACCOUNT', 'COLLECTION', 'HUB', 'TENANT'] as const;

export type ViewScope = (typeof VIEW_SCOPES)[number];

/** Whether saving at this scope is shaping the workspace, and therefore asks `STRUCTURE`. */
export function needsStructure(scope: ViewScope): boolean {
  return scope !== 'ACCOUNT';
}

/**
 * What a view stores as its query: a whole, anchored query — in the shape the **use case** takes.
 *
 * Two things the walk against a running server settled, and neither was guessable from the schema.
 *
 * The scope has to be there: `:export` runs the stored query and refuses one without it
 * (`query.scope_required`), so a view that stored only the question is a view nobody can export.
 *
 * And the scope is **flat**. `POST /items:query` takes `scope: {container_id}` as a request body,
 * and `presentation/rest` flattens it to `scope_container_id` before the use case sees it — which
 * is the form `ExportView` reads back out of the stored document. A view storing the *body* shape
 * saves without complaint and refuses at every export, because nothing reads it until then.
 */
export function queryDocumentOf(
  query: ItemsQuery | undefined,
  collectionId: string,
): Record<string, unknown> {
  return {
    scope_container_id: collectionId,
    include_descendants: false,
    ...(query?.filter ? { filter: query.filter } : {}),
    ...(query?.sort ? { sort: query.sort } : {}),
  };
}

/**
 * …and back — the question half, which is what a screen applies.
 *
 * The stored scope is deliberately left behind: the list is already looking at a collection, and
 * re-anchoring it to the one the view was saved in would make opening a view a navigation nobody
 * asked for.
 */
export function queryOf(
  stored: Record<string, unknown> | undefined,
  grouping: Record<string, unknown> | undefined,
): ItemsQuery {
  const group = grouping?.['field'] ? (grouping as ItemsQuery['group']) : undefined;
  return {
    ...(stored?.['filter'] ? { filter: stored['filter'] as ItemsQuery['filter'] } : {}),
    ...(stored?.['sort'] ? { sort: stored['sort'] as ItemsQuery['sort'] } : {}),
    ...(group ? { group } : {}),
  };
}

// --- the export ---------------------------------------------------------------------------------

export const EXPORT_FORMATS = ['CSV', 'JSON', 'ICS'] as const;

export type ExportFormat = (typeof EXPORT_FORMATS)[number];

const EXTENSIONS: Record<ExportFormat, string> = { CSV: 'csv', JSON: 'json', ICS: 'ics' };

/**
 * What to call the file.
 *
 * The view's name, made safe for a file system, and the extension of the format. The server names
 * one in `Content-Disposition` where it can, and this is what is used when it does not — a
 * download called `export` tells somebody nothing about which of their six views it is.
 */
export function exportFileName(name: string, format: ExportFormat): string {
  const safe = name
    .trim()
    .replace(/[\\/:*?"<>|]/g, '-')
    .replace(/\s+/g, '-')
    // A run of separators is one separator: "Q3 / planning" is `Q3-planning`, not `Q3---planning`.
    .replace(/-+/g, '-')
    .replace(/^-|-$/g, '')
    .slice(0, 80);
  return `${safe === '' ? 'view' : safe}.${EXTENSIONS[format]}`;
}

/**
 * Whether the answer is the first page of a larger one.
 *
 * The header is the whole of the honesty here: a result that reached `max_export_rows` is answered
 * whole *up to the cap*, and a client that handed the file over without saying so would hand over
 * something that looks complete. Any value but a plain `false` counts as true — a header that is
 * present at all is the server saying something happened.
 */
export function isTruncated(headers: ReadonlyMap<string, string>): boolean {
  const said = headers.get('export-truncated');
  return said !== undefined && said.toLowerCase() !== 'false';
}

// --- the feed -----------------------------------------------------------------------------------

/** Where a feed stands: serving, revoked, or serving nothing because its view is gone. */
export type FeedState = 'active' | 'revoked' | 'orphaned';

/**
 * A feed's state, in the order the questions matter.
 *
 * Revoked first: a revoked feed over a deleted view is revoked, and saying "it serves nothing"
 * about a token that already stopped working would be answering a question nobody asked.
 */
export function feedStateOf(feed: { revoked_at?: string | null; view_id?: string | null }): FeedState {
  if (feed.revoked_at) return 'revoked';
  return feed.view_id ? 'active' : 'orphaned';
}
