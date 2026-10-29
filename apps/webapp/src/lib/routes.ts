// SPDX-License-Identifier: Apache-2.0
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
  // The address the invitation mail links to (`DeliverNotification.go`). It falls through
  // `presentation/webui` to `index.html`, and this is the screen it points at.
  { name: 'redeem', pattern: '/redeem' },
  // Where the reset mail links. Signed out like `/redeem`, and for the same reason: somebody who
  // arrives here has no password, and asking for one would be asking for the thing the screen
  // exists to set.
  { name: 'reset', pattern: '/reset' },
  // Where the mail confirming a managed account's new address links (`SendAddressConfirmation.go`).
  // Signed in or not, like `/reset`: whoever holds the mailbox may open it from any device.
  { name: 'confirm-address', pattern: '/confirm-address' },
  // The redirect URI `cmd/server/main.go` derives and registers with the provider. The address is
  // the server's rather than this table's choice: nothing about where an authorization code comes
  // back may be taken from a request.
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
  // attack (`security.md` §5).
  { name: 'tokens', pattern: '/profile/tokens', area: 'profile' },
  // The rest of Your settings, which is a section since ADR-0065 decision 3: one screen per
  // question, each with an address of its own, all of them the reader's own and therefore
  // `profile` - the area the mobile shell ships in full (ADR-0032).
  { name: 'appearance', pattern: '/profile/appearance', area: 'profile' },
  { name: 'notifications', pattern: '/profile/notifications', area: 'profile' },
  { name: 'security', pattern: '/profile/security', area: 'profile' },
  { name: 'sessions', pattern: '/profile/sessions', area: 'profile' },
  { name: 'devices', pattern: '/profile/devices', area: 'profile' },
  { name: 'grants', pattern: '/profile/apps', area: 'profile' },
  // ADR-0032's administration area. Every route added under it is tagged as it is
  // added, and `router.test.ts` asserts that the tagged set is exactly the set under
  // `/administration` — so a screen added here without the tag, or tagged without living here,
  // fails rather than quietly shipping in the mobile shell.
  { name: 'administration', pattern: '/administration', area: 'administration' },
  { name: 'workspace-settings', pattern: '/administration/workspace', area: 'administration' },
  { name: 'presets', pattern: '/administration/presets', area: 'administration' },
  { name: 'people', pattern: '/administration/people', area: 'administration' },
  // Kept as addresses that moved (`movedTo`): the groups and the roles are tabs of *People* now.
  { name: 'groups', pattern: '/administration/groups', area: 'administration' },
  { name: 'permissions', pattern: '/administration/permissions', area: 'administration' },
  // The service accounts and the third-party apps: one screen, two tabs. Their old addresses
  // moved there (`movedTo`).
  { name: 'machines', pattern: '/administration/machines', area: 'administration' },
  { name: 'service-accounts', pattern: '/administration/service-accounts', area: 'administration' },
  { name: 'apps', pattern: '/administration/apps', area: 'administration' },
  { name: 'voice', pattern: '/administration/voice', area: 'administration' },
  { name: 'mail', pattern: '/administration/mail', area: 'administration' },
  // Where a mail provider sends the browser back after connecting a mailbox (ADR-0094): the mail
  // screen, which hands the code and the state to the API and then drops them from the address.
  { name: 'mail-oauth', pattern: '/administration/mail/oauth', area: 'administration' },
  { name: 'rules', pattern: '/administration/rules', area: 'administration' },
  // The rule editor: a rule drawn as a path. `new` before `:id`, so that a rule whose
  // identifier happened to be `new` could not exist and the two do not compete.
  { name: 'rule-new', pattern: '/administration/rules/new', area: 'administration' },
  { name: 'rule', pattern: '/administration/rules/:id', area: 'administration' },
  { name: 'runs', pattern: '/administration/runs', area: 'administration' },
  { name: 'webhooks', pattern: '/administration/webhooks', area: 'administration' },
  { name: 'quotas', pattern: '/administration/quotas', area: 'administration' },
  // The suite's modules, switched for the workspace (UC-INS-21).
  { name: 'modules', pattern: '/administration/modules', area: 'administration' },
  // The workspace's entry types, where the workflows module is on (UC-FLW-01 check 1).
  { name: 'entry-types', pattern: '/administration/types', area: 'administration' },
  { name: 'link-types', pattern: '/administration/link-types', area: 'administration' },
  { name: 'changes', pattern: '/administration/changes', area: 'administration' },
  // The workflow designer, where the workflows module is on (UC-FLW-02 check 2).
  { name: 'workflows', pattern: '/administration/workflows', area: 'administration' },
  // The private hubs, without their names (UC-ID-16 check 3).
  { name: 'private-hubs', pattern: '/administration/private-hubs', area: 'administration' },
  { name: 'backup', pattern: '/administration/backup', area: 'administration' },
  { name: 'retention', pattern: '/administration/retention', area: 'administration' },
  { name: 'restore', pattern: '/administration/restore', area: 'administration' },
  { name: 'audit', pattern: '/administration/audit', area: 'administration' },
  { name: 'privacy', pattern: '/administration/privacy', area: 'administration' },
  // Kept as an address that moved (`movedTo`): the providers are a tab of the sign-in screen now.
  { name: 'identity-provider', pattern: '/administration/identity-provider', area: 'administration' },
  // How people sign in: the rules, what the installation locked, and the one action that asks
  // everybody for a new password. Beside the provider rather than inside the workspace screen,
  // because ADR-0065 decision 3 is one screen per question and this is a question of its own.
  { name: 'sign-in-settings', pattern: '/administration/sign-in', area: 'administration' },
  // The links the sign-in footer shows: the same rule, but nothing about how somebody signs in.
  { name: 'legal', pattern: '/administration/legal', area: 'administration' },
  // The AI provider and the consent that lets it be used: where a workspace's content may
  // be sent is administration, exactly as the sign-in provider above it is.
  { name: 'ai', pattern: '/administration/ai', area: 'administration' },
  // No parameter, and that is the point: `/search` is a `POST` because a search term is content
  // and a query string travels through access logs, proxies and browser history. A route that
  // carried the term would undo that in the address bar (security.md §9, ADR-0018).
  { name: 'search', pattern: '/search' },
  // Where things arrive before they are work. End-user, because deciding what an arrival becomes
  // is the work rather than the administration of it. The workspace's own: drawn for somebody who
  // reads the workspace itself, and inside it each control only for the permission it needs.
  { name: 'jumble', pattern: '/jumble' },
  { name: 'trash', pattern: '/trash' },
  { name: 'archive', pattern: '/archive' },
  // The address the board's cards and the search results link to. An entry is
  // a thing with its own history, so it is a screen rather than a row somewhere.
  { name: 'item', pattern: '/items/:id' },
  { name: 'hub', pattern: '/hubs/:id' },
  // A hub's space and one of its pages (M3). A page's own address names only the page: it is linked
  // from a mention, a search hit or a backlink, which know the page and not its hub.
  { name: 'hub-pages', pattern: '/hubs/:id/pages' },
  { name: 'page', pattern: '/pages/:id' },
  // A hub's whiteboards and one of them (M4), addressed as a page is.
  { name: 'hub-whiteboards', pattern: '/hubs/:id/whiteboards' },
  { name: 'whiteboard', pattern: '/whiteboards/:id' },
  // A hub's assets (M6) by section, and one configuration item, addressed by itself because an
  // impact, a duplicate or a health problem links it without its hub.
  { name: 'hub-assets', pattern: '/hubs/:id/assets' },
  { name: 'hub-assets-section', pattern: '/hubs/:id/assets/:section' },
  { name: 'ci', pattern: '/cis/:id' },
  { name: 'collection', pattern: '/collections/:id' },
  // A place's settings, a section each (design-system.md §11.8): an address, so a section is
  // linked, reloaded and gone back to like any page. The bare path opens the first section.
  { name: 'hub-settings', pattern: '/hubs/:id/settings' },
  { name: 'hub-settings-section', pattern: '/hubs/:id/settings/:section' },
  { name: 'collection-settings', pattern: '/collections/:id/settings' },
  { name: 'collection-settings-section', pattern: '/collections/:id/settings/:section' },
  // The level above the workspaces (ADR-0070 §5). Its own area, excluded from the shells as
  // administration is: an administrator runs a workspace and an operator runs the installation, and
  // a shell that shipped one because it shipped the other would ship the control plane by accident.
  //
  // Not reachable without a raised session - which is the server's rule and not this table's: every
  // screen here reads a route behind `admin:tenants`, and a reader without the scope meets the
  // refusal rather than a list that is quietly empty.
  { name: 'instance', pattern: '/instance', area: 'instance' },
  { name: 'instance-workspaces', pattern: '/instance/workspaces', area: 'instance' },
  { name: 'instance-plans', pattern: '/instance/plans', area: 'instance' },
  { name: 'instance-plan', pattern: '/instance/plans/:name', area: 'instance' },
  { name: 'instance-settings', pattern: '/instance/settings', area: 'instance' },
  { name: 'instance-providers', pattern: '/instance/providers', area: 'instance' },
  { name: 'instance-ai', pattern: '/instance/ai', area: 'instance' },
  { name: 'instance-targets', pattern: '/instance/backup-targets', area: 'instance' },
  { name: 'instance-platform', pattern: '/instance/platform', area: 'instance' },
  { name: 'instance-encryption', pattern: '/instance/encryption', area: 'instance' },
  { name: 'instance-operators', pattern: '/instance/operators', area: 'instance' },
  { name: 'instance-journal', pattern: '/instance/journal', area: 'instance' },
];

/** Where the area's own screens live. One prefix, so the test and the table cannot disagree. */
export const ADMINISTRATION_PREFIX = '/administration';

/**
 * Addresses whose screen became a tab of another, by route name: each answers by replacing itself
 * with where its screen now is, so a bookmark or a link in a mail keeps working and the back button
 * does not return to the address that sent the reader on.
 */
const MOVED: Readonly<Record<string, string>> = {
  'identity-provider': '/administration/sign-in?tab=ways',
  groups: '/administration/people?tab=groups',
  permissions: '/administration/people?tab=permissions',
  'service-accounts': '/administration/machines',
  apps: '/administration/machines?tab=apps',
};

/** Where an address that moved now is, or nothing for one that did not. */
export function movedTo(name: string | null): string | undefined {
  return name === null ? undefined : MOVED[name];
}

/** The instance area's own prefix, for the same reason (ADR-0070 §5). */
export const INSTANCE_PREFIX = '/instance';

/** The routes that draw a hub or a collection: the place itself, and its settings (`ContainerView`). */
export const PLACE_ROUTES: readonly string[] = ['hub', 'collection', 'hub-settings', 'hub-settings-section', 'collection-settings', 'collection-settings-section'];

/** Whether a route is a place's settings, with or without its section named. */
export const isSettingsRoute = (name: string | null): boolean => name !== null && /-settings(-section)?$/u.test(name);

/**
 * What `/collections/:id?item=:itemId` means at a width (ADR-0061 decision 4): from `large` up,
 * the collection with the entry open in the detail pane beside its list; below `large`, the
 * entry's own page - the address the client redirects to, replacing the one it was given, so
 * the back button goes to where the reader came from. `/items/:id` is the entry's address on
 * every width and is not touched by this. Pure, so the redirect has a test without a browser.
 */
export type PaneResolution =
  | { readonly kind: 'pane'; readonly collectionId: string; readonly itemId: string }
  | { readonly kind: 'redirect'; readonly path: string }
  | { readonly kind: 'none' };

export function paneFor(
  route: { readonly name: string | null; readonly params: Readonly<Record<string, string>>; readonly query: Readonly<Record<string, string>> },
  options: { readonly isLarge: boolean },
): PaneResolution {
  const itemId = route.query['item'];
  const collectionId = route.params['id'];
  if (route.name !== 'collection' || !itemId || !collectionId) return { kind: 'none' };
  if (!options.isLarge) return { kind: 'redirect', path: `/items/${encodeURIComponent(itemId)}` };
  return { kind: 'pane', collectionId, itemId };
}
