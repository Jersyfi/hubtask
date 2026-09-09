// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The application's route table.
 *
 * Beside `router.ts` rather than inside `App.svelte` for one reason: ADR-0032's areas are a rule
 * about the *set* of routes — the mobile shell ships end-user and profile in full and reaches
 * administration through the web app — and a rule about a set can only be tested where the set can
 * be imported. A table declared inside a component could only be checked by mounting one.
 *
 * Ordered specific-first, because `resolve` takes the first match.
 */

import type { Route } from './router.ts';

export const ROUTES: readonly Route[] = [
  { name: 'home', pattern: '/' },
  // The address the invitation mail links to (`DeliverNotification.go`). It has fallen through
  // `presentation/webui` to `index.html` and resolved to nothing since H-01 shipped the mail;
  // this is the screen it was always pointing at.
  { name: 'redeem', pattern: '/redeem' },
  // The redirect URI `cmd/server/main.go` derives and registers with the provider. The address is
  // the server's rather than this table's choice: nothing about where an authorization code comes
  // back may be taken from a request (H-04).
  { name: 'oidc-callback', pattern: '/auth/callback' },
  // Where a third-party app sends somebody to be asked. `end-user`, because being asked whether
  // to allow an app is not administration — it is a decision every member makes for themselves.
  { name: 'consent', pattern: '/oauth/consent' },
  { name: 'installation', pattern: '/installation' },
  // ADR-0032's profile area, declared now rather than reclassified later: the mobile shell ships
  // this area in full and excludes administration, and a route that carried no area would be one
  // somebody has to classify by reading it.
  { name: 'profile', pattern: '/profile', area: 'profile' },
  // A person's own credentials are profile configuration, not administration: nobody else can
  // list them, and an administrator who could would learn which of somebody's automations to
  // attack (F4-10, `security.md` §5).
  { name: 'tokens', pattern: '/profile/tokens', area: 'profile' },
  // ADR-0032's administration area. Every route this milestone adds under it is tagged as it is
  // added, and `router.test.ts` asserts that the tagged set is exactly the set under
  // `/administration` — so a screen added here without the tag, or tagged without living here,
  // fails rather than quietly shipping in the mobile shell.
  { name: 'administration', pattern: '/administration', area: 'administration' },
  { name: 'workspace-settings', pattern: '/administration/workspace', area: 'administration' },
  { name: 'people', pattern: '/administration/people', area: 'administration' },
  { name: 'groups', pattern: '/administration/groups', area: 'administration' },
  { name: 'permissions', pattern: '/administration/permissions', area: 'administration' },
  { name: 'service-accounts', pattern: '/administration/service-accounts', area: 'administration' },
  { name: 'apps', pattern: '/administration/apps', area: 'administration' },
  { name: 'quotas', pattern: '/administration/quotas', area: 'administration' },
  { name: 'identity-provider', pattern: '/administration/identity-provider', area: 'administration' },
  // No parameter, and that is the point: `/search` is a `POST` because a search term is content
  // and a query string travels through access logs, proxies and browser history. A route that
  // carried the term would undo that in the address bar (security.md §9, ADR-0018).
  { name: 'search', pattern: '/search' },
  // Where things arrive before they are work. End-user, because deciding what an arrival becomes
  // is the work rather than the administration of it — only rotating the intake address needs
  // `AUTOMATION`, and the server refuses that on the screen.
  { name: 'jumble', pattern: '/jumble' },
  { name: 'trash', pattern: '/trash' },
  // The address the board's cards and the search results have linked to since F2-11. An entry is
  // a thing with its own history (F2-15), so it is a screen rather than a row somewhere.
  { name: 'item', pattern: '/items/:id' },
  { name: 'hub', pattern: '/hubs/:id' },
  { name: 'collection', pattern: '/collections/:id' },
];

/** Where the area's own screens live. One prefix, so the test and the table cannot disagree. */
export const ADMINISTRATION_PREFIX = '/administration';
