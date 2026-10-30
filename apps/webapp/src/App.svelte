<!-- SPDX-License-Identifier: Apache-2.0
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
  // else is a code rendered from `locales/en.json` (ADR-0011).
  import AppFrame from './lib/frame/AppFrame.svelte';
  import LazyView from './lib/frame/LazyView.svelte';
  import { messages, t } from './lib/i18n/i18n.svelte.ts';
  import { Router, type Resolution } from './lib/router.ts';
  import { PLACE_ROUTES, ROUTES, isSettingsRoute, movedTo, paneFor } from './lib/routes.ts';
  import { viewport } from './lib/frame/viewport.svelte.ts';
  import { actor } from './lib/data/account.svelte.ts';
  import { manifest } from './lib/data/capabilities.svelte.ts';
  import { holdsHere, reachesAdministrationArea } from './lib/data/capability.svelte.ts';
  import { live } from './lib/data/live.svelte.ts';
  import { platform } from './lib/platform/index.ts';
  import { session } from './lib/session.svelte.ts';
  import ContainerView from './views/ContainerView.svelte';
  import TodayView from './views/TodayView.svelte';
  import ItemView from './views/ItemView.svelte';
  import ConsentView from './views/ConsentView.svelte';
  import OidcCallbackView from './views/OidcCallbackView.svelte';
  import RedeemView from './views/RedeemView.svelte';
  import ResetView from './views/ResetView.svelte';
  import ConfirmAddressView from './views/ConfirmAddressView.svelte';
  import SetupView from './views/SetupView.svelte';
  import SignInView from './views/SignInView.svelte';

  // The table lives in `lib/routes.ts` so that ADR-0032's areas can be asserted as a set:
  // a rule about which routes belong to the administration area can only be tested where the
  // set can be imported, and a table declared here could only be checked by mounting this.
  // Route-level code splitting (F-10): every screen but the work's own - today, a place, an entry -
  // and the doors before the frame is its own chunk, loaded when its route is first reached.
  const views = {
    AdministrationOverviewView: () => import('./views/AdministrationOverviewView.svelte'),
    ArchiveView: () => import('./views/ArchiveView.svelte'),
    JumbleView: () => import('./views/JumbleView.svelte'),
    InstallationView: () => import('./views/InstallationView.svelte'),
    InstanceEncryptionView: () => import('./views/InstanceEncryptionView.svelte'),
    InstanceJournalView: () => import('./views/InstanceJournalView.svelte'),
    InstanceOperatorsView: () => import('./views/InstanceOperatorsView.svelte'),
    InstanceOverviewView: () => import('./views/InstanceOverviewView.svelte'),
    InstanceProvidersView: () => import('./views/InstanceProvidersView.svelte'),
    InstanceAiView: () => import('./views/InstanceAiView.svelte'),
    InstanceTargetsView: () => import('./views/InstanceTargetsView.svelte'),
    InstancePlatformView: () => import('./views/InstancePlatformView.svelte'),
    InstancePlanView: () => import('./views/InstancePlanView.svelte'),
    InstancePlansView: () => import('./views/InstancePlansView.svelte'),
    InstanceSettingsView: () => import('./views/InstanceSettingsView.svelte'),
    InstanceWorkspacesView: () => import('./views/InstanceWorkspacesView.svelte'),
    ProfileView: () => import('./views/ProfileView.svelte'),
    AppearanceView: () => import('./views/AppearanceView.svelte'),
    NotificationsView: () => import('./views/NotificationsView.svelte'),
    SecurityView: () => import('./views/SecurityView.svelte'),
    SessionsView: () => import('./views/SessionsView.svelte'),
    DevicesView: () => import('./views/DevicesView.svelte'),
    GrantsView: () => import('./views/GrantsView.svelte'),
    SearchView: () => import('./views/SearchView.svelte'),
    MyTokensView: () => import('./views/MyTokensView.svelte'),
    TrashView: () => import('./views/TrashView.svelte'),
    MachinesView: () => import('./views/MachinesView.svelte'),
    PeopleView: () => import('./views/PeopleView.svelte'),
    QuotasView: () => import('./views/QuotasView.svelte'),
    PrivateHubsView: () => import('./views/PrivateHubsView.svelte'),
    ModulesView: () => import('./views/ModulesView.svelte'),
    EntryTypesView: () => import('./views/EntryTypesView.svelte'),
    LinkTypesView: () => import('./views/LinkTypesView.svelte'),
    CmdbView: () => import('./views/CmdbView.svelte'),
    PagesView: () => import('./views/PagesView.svelte'),
    WhiteboardsView: () => import('./views/WhiteboardsView.svelte'),
    PlanView: () => import('./views/PlanView.svelte'),
    SprintView: () => import('./views/SprintView.svelte'),
    AssetsView: () => import('./views/AssetsView.svelte'),
    ChangesView: () => import('./views/ChangesView.svelte'),
    WorkflowsView: () => import('./views/WorkflowsView.svelte'),
    BackupView: () => import('./views/BackupView.svelte'),
    RetentionView: () => import('./views/RetentionView.svelte'),
    RestoreView: () => import('./views/RestoreView.svelte'),
    AuditView: () => import('./views/AuditView.svelte'),
    PrivacyView: () => import('./views/PrivacyView.svelte'),
    RulesView: () => import('./views/RulesView.svelte'),
    RuleEditorView: () => import('./views/RuleEditorView.svelte'),
    RunsView: () => import('./views/RunsView.svelte'),
    WebhooksView: () => import('./views/WebhooksView.svelte'),
    VoiceView: () => import('./views/VoiceView.svelte'),
    MailView: () => import('./views/MailView.svelte'),
    WorkspaceSettingsView: () => import('./views/WorkspaceSettingsView.svelte'),
    PresetsView: () => import('./views/PresetsView.svelte'),
    AiSettingsView: () => import('./views/AiSettingsView.svelte'),
    SignInSettingsView: () => import('./views/SignInSettingsView.svelte'),
  };

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
  // An address whose screen became a tab elsewhere (`movedTo`) replaces itself with that tab's.
  const moved = $derived(movedTo(route.name));
  $effect(() => {
    if (moved) router.replace(moved);
  });

  /**
   * Whether the reader is inside the level this address belongs to (P-05: a level somebody is
   * outside of does not appear at all). The menu does not offer the administration to somebody
   * who does not reach it, nor the jumble to somebody who does not read the workspace itself; an
   * address typed or kept from an earlier role must not offer either - their forms, their *Save*,
   * and the reads behind them that the server refuses and records. Outside,
   * the address answers as one that reaches nothing; `undefined` while the manifest has not yet
   * answered for this reader, and then nothing is drawn rather than a not-found that turns into a
   * screen a moment later.
   */
  const reach = $derived(
    route.area === 'administration'
      ? reachesAdministrationArea()
      : route.name === 'jumble'
        ? holdsHere('READ')
        : true,
  );

  /**
   * The one place the language is decided, because it is the one place that knows both halves:
   * what the reader prefers (their account, then their browser) and what the installation has
   * (the manifest). `i18n-l10n.md` §2's order, with the parenthesis that inverts its top - the
   * account wins over `Accept-Language`, which is what answers before there is an account.
   *
   * Here rather than in the frame, because the signed-out card is not inside the frame: decided
   * there, the card, the reset and the provider's return would render the source language whatever
   * the browser asked for, which is exactly the half of the table that speaks before an account does.
   *
   * It runs again whenever either half changes, which is what makes the manifest's arrival turn
   * the document round on an installation that serves a right-to-left locale.
   */
  $effect(() => {
    messages.adopt(
      { account: actor.locale, requested: navigator.languages },
      manifest.supportedLocales,
    );
  });

  /**
   * The address the sign-in card is reached at when somebody types it or follows a link to it.
   *
   * Deliberately not a route: the card is drawn for anybody who is signed out, wherever they are,
   * which is what makes a deep link survive an expired session. This constant exists for the one
   * place that has to know the difference - the moment after a sign-in, when the card stops being
   * drawn and this address resolves to nothing.
   */
  const signInPath = '/sign-in';

  // Signing in again returns the reader to what they were looking at when the session ended. The
  // path is taken once: one that navigated twice would fight the reader's next click.
  //
  // And where there is none, the overview. `/sign-in` is an address the product prints itself -
  // in a reset mail's neighbour, in the card's own links - and it is deliberately *not* a route:
  // the card is drawn for anybody who is signed out, wherever they are. Which means that the
  // moment the sign-in succeeds the card stops being drawn and the address resolves to nothing,
  // so without this line a successful sign-in ends on the frame's "nothing at this address" -
  // the one place it must not.
  $effect(() => {
    if (!session.isSignedIn) return;
    const intended = session.takeIntendedPath() ?? (route.path === signInPath ? '/' : undefined);
    if (intended && intended !== route.path) router.navigate(intended);
  });

  /**
   * The one stream this tab keeps, opened once there is a credential to open it with and an
   * account to hold the copy for - the replica is one store per account, so the stream
   * waits for `/accounts/me` rather than for the bearer alone.
   *
   * Here rather than in a view, because it belongs to the session rather than to a screen: a
   * reader who navigates from a board to an entry does not want the connection torn down and made
   * again. `live.stop()` is called by the sign-out itself, so the teardown here is only for a tab
   * that closes.
   *
   * The effect follows the account's **id** and nothing else about it. Reading `actor.account`
   * directly would re-run it on every state the account passes through - and with the server
   * away, each re-run tears the stream down and attaches the store again, each attach reads the
   * failed account again, each failure is a new state: a loop as tight as the network lets it be,
   * hundreds of reads of `/accounts/me` in the seconds an outage lasts.
   */
  const liveAccountId = $derived(
    // The account from `/accounts/me`, or the one remembered beside the pair when the server
    // cannot be reached: a tab reloading offline still opens its replica.
    session.isSignedIn ? (actor.account?.id ?? platform.lastAccount()) : undefined,
  );
  $effect(() => {
    const accountId = liveAccountId;
    if (!accountId) return;
    live.start(accountId);
    return () => live.stop();
  });
</script>

<!-- Signed out there is no shell, and that is the decision rather than an omission (ADR-0061 draws
     the bar for a page that has navigation; this one has none). The card is the page: it brings
     its own `main`, its own canvas and the footer the operator's legal links live in, so a screen
     nobody can navigate away from is not wrapped in the furniture of navigating.
     The two link screens are here whether or not somebody is signed in: an invitation and a reset
     link are proof about an account that need not be the one in this tab, and a person who opened
     one from their mail while signed in would otherwise meet "there is nothing at this address". -->
{#if route.name === 'oidc-callback'}
  <!-- The provider's return, on the card and never in the frame (UC-ID-08 check 5): this address
       is reached by a redirect into a fresh document, before there is a session, and the frame of
       an application nobody is signed into is furniture around one sentence. Whether or not a
       session exists: the moment the exchange opens one, this screen sends the reader on, and a
       second copy of it mounted inside the frame would find the address already cleaned and
       report a refusal for a sign-in that succeeded. -->
  <OidcCallbackView onnavigate={(path) => router.navigate(path)} />
{:else if !session.isSignedIn || route.name === 'reset' || route.name === 'redeem' || route.name === 'confirm-address'}
  {#if route.name === 'reset'}
    <!-- Before the sign-in screen, exactly as the invitation is: somebody who arrives with a
         reset link is here to set a password, not to remember the one they forgot. -->
    <ResetView onnavigate={(path) => router.navigate(path)} />
  {:else if route.name === 'confirm-address'}
    <!-- Signed in or not: whoever holds the mailbox confirms, from whichever device the mail is
         read on. It signs nobody in. -->
    <ConfirmAddressView onnavigate={(path) => router.navigate(path)} />
  {:else if route.name === 'redeem'}
    <!-- Before the sign-in screen: somebody arriving with an invitation has no password yet, and
         asking them for one would be asking for the thing this screen exists to set. -->
    <RedeemView onnavigate={(path) => router.navigate(path)} />
  {:else if manifest.isSetupOpen}
    <!-- A fresh installation: nothing to sign in to yet, so the first start's setup takes the
         sign-in card's place until the first workspace exists (UC-INS-01). -->
    <SetupView onnavigate={(path) => router.navigate(path)} />
  {:else}
    <SignInView />
  {/if}
{:else}
<AppFrame {route} onnavigate={(path) => router.navigate(path)}>
  {#if reach === undefined}
    <!-- Not yet known whether this reader is inside the level: nothing, rather than a guess. -->
  {:else if reach === false}
    {@render nothingHere()}
  {:else if route.name === 'consent'}
    <!-- Signed in only: `POST /oauth/authorize` needs a person, never a token, so somebody who
         arrives here signed out meets the sign-in screen first and lands back on this address. -->
    <ConsentView />
  {:else if route.name === 'home'}
    <TodayView onnavigate={(path) => router.navigate(path)} />
  {:else if route.name === 'installation'}
    <LazyView load={views.InstallationView} />
  {:else if route.name === 'profile'}
    <LazyView load={views.ProfileView} />
  {:else if route.name === 'tokens'}
    <LazyView load={views.MyTokensView} />
  {:else if route.name === 'appearance'}
    <LazyView load={views.AppearanceView} />
  {:else if route.name === 'notifications'}
    <LazyView load={views.NotificationsView} />
  {:else if route.name === 'security'}
    <LazyView load={views.SecurityView} />
  {:else if route.name === 'sessions'}
    <LazyView load={views.SessionsView} />
  {:else if route.name === 'devices'}
    <LazyView load={views.DevicesView} />
  {:else if route.name === 'grants'}
    <LazyView load={views.GrantsView} />
  {:else if route.name === 'instance'}
    <!-- The level above the workspaces (ADR-0070 §5). Its own area, and its own door: every screen
         here is drawn only while the session is raised, and the hour's end returns the reader to
         the application rather than leaving them looking at numbers nothing will refresh. -->
    <LazyView load={views.InstanceOverviewView} props={{ onnavigate: (path: string) => router.navigate(path) }} />
  {:else if route.name === 'instance-workspaces'}
    <LazyView load={views.InstanceWorkspacesView} props={{ onnavigate: (path: string) => router.navigate(path) }} />
  {:else if route.name === 'instance-plans'}
    <LazyView load={views.InstancePlansView} props={{ onnavigate: (path: string) => router.navigate(path) }} />
  {:else if route.name === 'instance-plan'}
    {#key route.params.name}
      <LazyView load={views.InstancePlanView} props={{ name: route.params.name ?? '', onnavigate: (path: string) => router.navigate(path) }} />
    {/key}
  {:else if route.name === 'instance-settings'}
    <LazyView load={views.InstanceSettingsView} props={{ tab: route.query['tab'], onnavigate: (path: string) => router.navigate(path), onreplace: (path: string) => router.replace(path) }} />
  {:else if route.name === 'instance-providers'}
    <LazyView load={views.InstanceProvidersView} props={{ onnavigate: (path: string) => router.navigate(path) }} />
  {:else if route.name === 'instance-ai'}
    <LazyView load={views.InstanceAiView} props={{ onnavigate: (path: string) => router.navigate(path) }} />
  {:else if route.name === 'instance-targets'}
    <LazyView load={views.InstanceTargetsView} props={{ onnavigate: (path: string) => router.navigate(path) }} />
  {:else if route.name === 'instance-platform'}
    <LazyView load={views.InstancePlatformView} props={{ onnavigate: (path: string) => router.navigate(path) }} />
  {:else if route.name === 'instance-encryption'}
    <LazyView load={views.InstanceEncryptionView} props={{ onnavigate: (path: string) => router.navigate(path) }} />
  {:else if route.name === 'instance-operators'}
    <LazyView load={views.InstanceOperatorsView} props={{ onnavigate: (path: string) => router.navigate(path) }} />
  {:else if route.name === 'instance-journal'}
    <LazyView load={views.InstanceJournalView} props={{ onnavigate: (path: string) => router.navigate(path) }} />
  {:else if route.name === 'administration'}
    <LazyView load={views.AdministrationOverviewView} />
  {:else if route.name === 'workspace-settings'}
    <LazyView load={views.WorkspaceSettingsView} />
  {:else if route.name === 'presets'}
    <LazyView load={views.PresetsView} />
  {:else if route.name === 'people'}
    <LazyView load={views.PeopleView} props={{ tab: route.query['tab'], person: route.query['person'], onnavigate: (path: string) => router.replace(path) }} />
  {:else if route.name === 'machines'}
    <LazyView load={views.MachinesView} props={{ tab: route.query['tab'], onnavigate: (path: string) => router.replace(path) }} />
  {:else if route.name === 'rules'}
    <LazyView load={views.RulesView} props={{ onnavigate: (path: string) => router.navigate(path) }} />
  {:else if route.name === 'rule-new'}
    <LazyView load={views.RuleEditorView} props={{ id: 'new', workflow: route.query['workflow'], step: route.query['step'], onnavigate: (path: string) => router.navigate(path) }} />
  {:else if route.name === 'rule'}
    {#key route.params.id}
      <LazyView load={views.RuleEditorView} props={{ id: route.params.id ?? '', onnavigate: (path: string) => router.navigate(path) }} />
    {/key}
  {:else if route.name === 'runs'}
    <LazyView load={views.RunsView} />
  {:else if route.name === 'webhooks'}
    <LazyView load={views.WebhooksView} />
  {:else if route.name === 'quotas'}
    <LazyView load={views.QuotasView} />
  {:else if route.name === 'modules'}
    <LazyView load={views.ModulesView} />
  {:else if route.name === 'entry-types'}
    <LazyView load={views.EntryTypesView} />
  {:else if route.name === 'link-types'}
    <LazyView load={views.LinkTypesView} />
  {:else if route.name === 'cmdb'}
    <LazyView load={views.CmdbView} />
  {:else if route.name === 'changes'}
    <LazyView load={views.ChangesView} />
  {:else if route.name === 'workflows'}
    <LazyView load={views.WorkflowsView} props={{ onnavigate: (path: string) => router.navigate(path) }} />
  {:else if route.name === 'private-hubs'}
    <LazyView load={views.PrivateHubsView} props={{ onnavigate: (path: string) => router.navigate(path) }} />
  {:else if route.name === 'backup'}
    <LazyView load={views.BackupView} />
  {:else if route.name === 'retention'}
    <LazyView load={views.RetentionView} />
  {:else if route.name === 'restore'}
    <LazyView load={views.RestoreView} />
  {:else if route.name === 'audit'}
    <LazyView load={views.AuditView} />
  {:else if route.name === 'privacy'}
    <LazyView load={views.PrivacyView} />
  {:else if movedTo(route.name)}
    <!-- Nothing for the moment before the address replaces itself, rather than a not-found. -->
  {:else if route.name === 'sign-in-settings'}
    <LazyView load={views.SignInSettingsView} props={{ tab: route.query['tab'], onnavigate: (path: string) => router.replace(path) }} />
  {:else if route.name === 'voice'}
    <LazyView load={views.VoiceView} />
  {:else if route.name === 'mail' || route.name === 'mail-oauth'}
    <LazyView load={views.MailView} props={{ providerAnswer: route.name === 'mail-oauth' ? route.query : undefined, onanswered: () => router.replace('/administration/mail') }} />
  {:else if route.name === 'legal'}
    <LazyView load={views.SignInSettingsView} props={{ part: 'legal' }} />
  {:else if route.name === 'ai'}
    <LazyView load={views.AiSettingsView} props={{ tab: route.query['tab'], onnavigate: (path: string) => router.replace(path) }} />
  {:else if route.name === 'search'}
    <LazyView load={views.SearchView} props={{ query: route.query, onnavigate: (path: string) => router.replace(path) }} />
  {:else if route.name === 'jumble'}
    <LazyView load={views.JumbleView} props={{ onnavigate: (path: string) => router.navigate(path) }} />
  {:else if route.name === 'trash'}
    <LazyView load={views.TrashView} />
  {:else if route.name === 'archive'}
    <LazyView load={views.ArchiveView} props={{ onnavigate: (path: string) => router.navigate(path) }} />
  {:else if route.name === 'item'}
    {#key route.params.id}
      <ItemView id={route.params.id ?? ''} onnavigate={(path) => router.navigate(path)} />
    {/key}
  {:else if route.name === 'hub-pages' || route.name === 'page'}
    {#key route.path}
      <LazyView
        load={views.PagesView}
        props={{
          hubId: route.name === 'hub-pages' ? route.params.id : undefined,
          pageId: route.name === 'page' ? route.params.id : undefined,
          onnavigate: (path: string) => router.navigate(path),
        }}
      />
    {/key}
  {:else if route.name === 'hub-whiteboards' || route.name === 'whiteboard'}
    {#key route.path}
      <LazyView
        load={views.WhiteboardsView}
        props={{
          hubId: route.name === 'hub-whiteboards' ? route.params.id : undefined,
          boardId: route.name === 'whiteboard' ? route.params.id : undefined,
          onnavigate: (path: string) => router.navigate(path),
        }}
      />
    {/key}
  {:else if route.name === 'hub-plan' || route.name === 'collection-plan'}
    {#key route.path}
      <LazyView load={views.PlanView} props={{ planId: route.params.id, onnavigate: (path: string) => router.navigate(path) }} />
    {/key}
  {:else if route.name === 'collection-sprints'}
    {#key route.path}
      <LazyView load={views.SprintView} props={{ collectionId: route.params.id, onnavigate: (path: string) => router.navigate(path) }} />
    {/key}
  {:else if route.name === 'hub-assets' || route.name === 'hub-assets-section' || route.name === 'ci'}
    {#key route.name === 'ci' ? route.path : route.params.id}
      <LazyView
        load={views.AssetsView}
        props={{
          hubId: route.name === 'ci' ? undefined : route.params.id,
          section: route.name === 'hub-assets-section' ? route.params.section : undefined,
          ciId: route.name === 'ci' ? route.params.id : undefined,
          onnavigate: (path: string) => router.navigate(path),
        }}
      />
    {/key}
  {:else if PLACE_ROUTES.includes(route.name ?? '')}
    <!-- One view for both: they differ in what they hold, not in what they are. Keyed on the id so
         that navigating from one collection to another rebuilds rather than reusing the state of
         the one before — a draft rename would otherwise follow the reader to a different name. -->
    {#key route.params.id}
      <ContainerView
        id={route.params.id ?? ''}
        openItemId={pane.kind === 'pane' ? pane.itemId : undefined}
        section={isSettingsRoute(route.name) ? (route.params.section ?? '') : undefined}
        onnavigate={(path) => router.navigate(path)}
      />
    {/key}
  {:else}
    {@render nothingHere()}
  {/if}
</AppFrame>
{/if}

{#snippet nothingHere()}
  <!-- The server's own code for a path that reaches nothing, at the same address - as the
       page's heading, because a screen without one is a screen a reader cannot name. -->
  <h1>{t('route.unknown')}</h1>
  <p><code>{route.path}</code></p>
  <p><a href="/">{t('app.back_to_start')}</a></p>
{/snippet}
