<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The root: the route table, and the frame every view is rendered inside.
  //
  // The table is real paths over the History API rather than `#/` (ADR-0028): the server's
  // `index.html` fallback exists so that a deep link survives a reload, and a fragment would make
  // that fallback pointless. Past the fallback the application owns its own paths, which is why an
  // unknown one renders the not-found view below rather than a 404.
  //
  // No sentence is written in this file. `Hubtask` is a name rather than a message, and everything
  // else is a code rendered from `locales/en.json` (ADR-0011, F1-07).
  import AppFrame from './lib/frame/AppFrame.svelte';
  import { t } from './lib/i18n/i18n.svelte.ts';
  import { Router, type Resolution } from './lib/router.ts';
  import { ROUTES, paneFor } from './lib/routes.ts';
  import { ADMINISTRATION, firstScreen } from './lib/navigation.ts';
  import { viewport } from './lib/frame/viewport.svelte.ts';
  import { actor } from './lib/data/account.svelte.ts';
  import { live } from './lib/data/live.svelte.ts';
  import { platform } from './lib/platform/index.ts';
  import { session } from './lib/session.svelte.ts';
  import ArchiveView from './views/ArchiveView.svelte';
import ContainerView from './views/ContainerView.svelte';
  import HomeView from './views/HomeView.svelte';
  import ItemView from './views/ItemView.svelte';
  import JumbleView from './views/JumbleView.svelte';
  import InstallationView from './views/InstallationView.svelte';
  import ProfileView from './views/ProfileView.svelte';
  import SearchView from './views/SearchView.svelte';
  import MyTokensView from './views/MyTokensView.svelte';
  import TrashView from './views/TrashView.svelte';
  import AppsView from './views/AppsView.svelte';
  import ConsentView from './views/ConsentView.svelte';
  import GroupsView from './views/GroupsView.svelte';
  import PeopleView from './views/PeopleView.svelte';
  import PermissionsView from './views/PermissionsView.svelte';
  import ServiceAccountsView from './views/ServiceAccountsView.svelte';
  import QuotasView from './views/QuotasView.svelte';
  import BackupView from './views/BackupView.svelte';
  import RetentionView from './views/RetentionView.svelte';
  import RestoreView from './views/RestoreView.svelte';
  import AuditView from './views/AuditView.svelte';
  import PrivacyView from './views/PrivacyView.svelte';
  import RulesView from './views/RulesView.svelte';
  import RuleEditorView from './views/RuleEditorView.svelte';
  import RunsView from './views/RunsView.svelte';
  import WebhooksView from './views/WebhooksView.svelte';
  import WorkspaceSettingsView from './views/WorkspaceSettingsView.svelte';
  import AiSettingsView from './views/AiSettingsView.svelte';
  import IdentityProviderView from './views/IdentityProviderView.svelte';
  import OidcCallbackView from './views/OidcCallbackView.svelte';
  import RedeemView from './views/RedeemView.svelte';
  import SignInView from './views/SignInView.svelte';

  // The table lives in `lib/routes.ts` so that ADR-0032's areas can be asserted as a set:
  // a rule about which routes belong to the administration area can only be tested where the
  // set can be imported, and a table declared here could only be checked by mounting this.
  const router = new Router(ROUTES);
  let route = $state<Resolution>(router.current);

  $effect(() => {
    const unsubscribe = router.subscribe((resolution) => (route = resolution));
    const stop = router.start();
    return () => {
      unsubscribe();
      stop();
    };
  });

  /**
   * The detail pane's address (ADR-0061 decision 4): `/collections/:id?item=:itemId` is the list
   * with the entry beside it from `large` up, and below `large` the entry's own page - a redirect
   * that replaces the address, so the back button goes to where the reader came from. Width, not
   * platform: the desktop shell dragged narrow redirects like a phone. `paneFor` is the pure
   * answer and has the test; this is only the effect that acts on it.
   */
  const pane = $derived(paneFor(route, { isLarge: viewport.isLarge }));
  $effect(() => {
    if (pane.kind === 'redirect') router.replace(pane.path);
  });

  /**
   * A section's own address opens its first screen (ADR-0065 decision 1).
   *
   * `/administration` is linked from the account menu and from the trail of every screen under it,
   * so it keeps its address; what it no longer has is an index, which was the column's list drawn
   * a second time. `replace` rather than `navigate`, for the reason the pane's redirect uses it:
   * the address the reader came from is the one the back button should return to, not the door
   * they were sent through.
   */
  $effect(() => {
    if (session.isSignedIn && route.name === 'administration') router.replace(firstScreen(ADMINISTRATION));
  });

  // Signing in again returns the reader to what they were looking at when the session ended. The
  // path is taken once: one that navigated twice would fight the reader's next click.
  $effect(() => {
    if (!session.isSignedIn) return;
    const intended = session.takeIntendedPath();
    if (intended && intended !== route.path) router.navigate(intended);
  });

  /**
   * The one stream this tab keeps, opened once there is a credential to open it with and an
   * account to hold the copy for - the replica is one store per account (F6-03), so the stream
   * waits for `/accounts/me` rather than for the bearer alone.
   *
   * Here rather than in a view, because it belongs to the session rather than to a screen: a
   * reader who navigates from a board to an entry does not want the connection torn down and made
   * again. `live.stop()` is called by the sign-out itself, so the teardown here is only for a tab
   * that closes.
   *
   * The effect follows the account's **id** and nothing else about it (issue 881). It used to read
   * `actor.account` directly, and so re-ran on every state the account passed through - and with
   * the server away, each re-run tore the stream down and attached the store again, each attach
   * read the failed account again, each failure was a new state: a loop as tight as the network
   * let it be, nine hundred reads of `/accounts/me` in the seconds an outage lasted in the walk.
   */
  const liveAccountId = $derived(
    // The account from `/accounts/me`, or the one remembered beside the pair when the server
    // cannot be reached: a tab reloading offline still opens its replica (F6-04).
    session.isSignedIn ? (actor.account?.id ?? platform.lastAccount()) : undefined,
  );
  $effect(() => {
    const accountId = liveAccountId;
    if (!accountId) return;
    live.start(accountId);
    return () => live.stop();
  });
</script>

<AppFrame {route} onnavigate={(path) => router.navigate(path)}>
  <!-- Nothing here is usable without a credential, and the token screen is what asks for one. The
       route is left alone while it is shown, so that the address the reader arrived at is still
       the address they land on afterwards. -->
  {#if route.name === 'oidc-callback'}
    <!-- Before both, and without asking whether there is a session: this address is reached by a
         provider's redirect into a fresh document, and the screen's own business is finishing that
         exchange. It sends the reader on once there is a session. -->
    <OidcCallbackView onnavigate={(path) => router.navigate(path)} />
  {:else if session.isSignedIn && route.name === 'consent'}
    <!-- Signed in only: `POST /oauth/authorize` needs a person, never a token, so somebody who
         arrives here signed out meets the sign-in screen first and lands back on this address. -->
    <ConsentView />
  {:else if !session.isSignedIn && route.name === 'redeem'}
    <!-- Before the sign-in screen: somebody arriving with an invitation has no password yet, and
         asking them for one would be asking for the thing this screen exists to set. -->
    <RedeemView onnavigate={(path) => router.navigate(path)} />
  {:else if !session.isSignedIn}
    <SignInView />
  {:else if route.name === 'home'}
    <HomeView onnavigate={(path) => router.navigate(path)} />
  {:else if route.name === 'installation'}
    <InstallationView />
  {:else if route.name === 'profile'}
    <ProfileView />
  {:else if route.name === 'tokens'}
    <MyTokensView />
  {:else if route.name === 'administration'}
    <!-- The section's front door (ADR-0065 decision 1). The effect above replaces the address with
         the section's first screen, so nothing is drawn here - not even for the tick in between,
         which is what a screen saying "not found" under a real address would be. -->
  {:else if route.name === 'workspace-settings'}
    <WorkspaceSettingsView />
  {:else if route.name === 'people'}
    <PeopleView />
  {:else if route.name === 'groups'}
    <GroupsView />
  {:else if route.name === 'permissions'}
    <PermissionsView />
  {:else if route.name === 'service-accounts'}
    <ServiceAccountsView />
  {:else if route.name === 'apps'}
    <AppsView />
  {:else if route.name === 'rules'}
    <RulesView onnavigate={(path) => router.navigate(path)} />
  {:else if route.name === 'rule-new'}
    <RuleEditorView id="new" onnavigate={(path) => router.navigate(path)} />
  {:else if route.name === 'rule'}
    {#key route.params.id}
      <RuleEditorView id={route.params.id ?? ''} onnavigate={(path) => router.navigate(path)} />
    {/key}
  {:else if route.name === 'runs'}
    <RunsView />
  {:else if route.name === 'webhooks'}
    <WebhooksView />
  {:else if route.name === 'quotas'}
    <QuotasView />
  {:else if route.name === 'backup'}
    <BackupView />
  {:else if route.name === 'retention'}
    <RetentionView />
  {:else if route.name === 'restore'}
    <RestoreView />
  {:else if route.name === 'audit'}
    <AuditView />
  {:else if route.name === 'privacy'}
    <PrivacyView />
  {:else if route.name === 'identity-provider'}
    <IdentityProviderView />
  {:else if route.name === 'ai'}
    <AiSettingsView />
  {:else if route.name === 'search'}
    <SearchView query={route.query} onnavigate={(path) => router.replace(path)} />
  {:else if route.name === 'jumble'}
    <JumbleView onnavigate={(path) => router.navigate(path)} />
  {:else if route.name === 'trash'}
    <TrashView />
  {:else if route.name === 'archive'}
    <ArchiveView onnavigate={(path) => router.navigate(path)} />
  {:else if route.name === 'item'}
    {#key route.params.id}
      <ItemView id={route.params.id ?? ''} onnavigate={(path) => router.navigate(path)} />
    {/key}
  {:else if route.name === 'hub' || route.name === 'collection'}
    <!-- One view for both: they differ in what they hold, not in what they are. Keyed on the id so
         that navigating from one collection to another rebuilds rather than reusing the state of
         the one before — a draft rename would otherwise follow the reader to a different name. -->
    {#key route.params.id}
      <ContainerView id={route.params.id ?? ''} openItemId={pane.kind === 'pane' ? pane.itemId : undefined} onnavigate={(path) => router.navigate(path)} />
    {/key}
  {:else}
    <!-- The server's own code for a path that reaches nothing, at the same address - as the
         page's heading, because a screen without one is a screen a reader cannot name (F5-11). -->
    <h1>{t('route.unknown')}</h1>
    <p><code>{route.path}</code></p>
    <p><a href="/">{t('app.back_to_start')}</a></p>
  {/if}
</AppFrame>
