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
  import { ROUTES } from './lib/routes.ts';
  import { live } from './lib/data/live.svelte.ts';
  import { session } from './lib/session.svelte.ts';
  import ContainerView from './views/ContainerView.svelte';
  import HomeView from './views/HomeView.svelte';
  import ItemView from './views/ItemView.svelte';
  import InstallationView from './views/InstallationView.svelte';
  import ProfileView from './views/ProfileView.svelte';
  import SearchView from './views/SearchView.svelte';
  import MyTokensView from './views/MyTokensView.svelte';
  import TrashView from './views/TrashView.svelte';
  import AdministrationView from './views/AdministrationView.svelte';
  import AppsView from './views/AppsView.svelte';
  import ConsentView from './views/ConsentView.svelte';
  import GroupsView from './views/GroupsView.svelte';
  import PeopleView from './views/PeopleView.svelte';
  import PermissionsView from './views/PermissionsView.svelte';
  import ServiceAccountsView from './views/ServiceAccountsView.svelte';
  import QuotasView from './views/QuotasView.svelte';
  import WorkspaceSettingsView from './views/WorkspaceSettingsView.svelte';
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

  // Signing in again returns the reader to what they were looking at when the session ended. The
  // path is taken once: one that navigated twice would fight the reader's next click.
  $effect(() => {
    if (!session.isSignedIn) return;
    const intended = session.takeIntendedPath();
    if (intended && intended !== route.path) router.navigate(intended);
  });

  /**
   * The one stream this tab keeps, opened once there is a credential to open it with.
   *
   * Here rather than in a view, because it belongs to the session rather than to a screen: a
   * reader who navigates from a board to an entry does not want the connection torn down and made
   * again. `live.stop()` is called by the sign-out itself, so the teardown here is only for a tab
   * that closes.
   */
  $effect(() => {
    if (!session.isSignedIn) return;
    live.start();
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
    <RedeemView />
  {:else if !session.isSignedIn}
    <SignInView />
  {:else if route.name === 'home'}
    <HomeView />
  {:else if route.name === 'installation'}
    <InstallationView />
  {:else if route.name === 'profile'}
    <ProfileView />
  {:else if route.name === 'tokens'}
    <MyTokensView />
  {:else if route.name === 'administration'}
    <AdministrationView />
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
  {:else if route.name === 'quotas'}
    <QuotasView />
  {:else if route.name === 'identity-provider'}
    <IdentityProviderView />
  {:else if route.name === 'search'}
    <SearchView />
  {:else if route.name === 'trash'}
    <TrashView />
  {:else if route.name === 'item'}
    {#key route.params.id}
      <ItemView id={route.params.id ?? ''} />
    {/key}
  {:else if route.name === 'hub' || route.name === 'collection'}
    <!-- One view for both: they differ in what they hold, not in what they are. Keyed on the id so
         that navigating from one collection to another rebuilds rather than reusing the state of
         the one before — a draft rename would otherwise follow the reader to a different name. -->
    {#key route.params.id}
      <ContainerView id={route.params.id ?? ''} onnavigate={(path) => router.navigate(path)} />
    {/key}
  {:else}
    <!-- The server's own code for a path that reaches nothing, at the same address. -->
    <p>{t('route.unknown')} <code>{route.path}</code></p>
    <p><a href="/">{t('app.back_to_start')}</a></p>
  {/if}
</AppFrame>
