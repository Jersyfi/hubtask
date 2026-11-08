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
  import { untrack } from 'svelte';
  import { Spinner } from '@hubtask/design-system/components';
  import type { ConnectorLocated } from '@hubtask/sync-engine';

  import LazyFrame from './lib/frame/LazyFrame.svelte';
  import LazyView from './lib/frame/LazyView.svelte';
  import ViewFailed from './lib/frame/ViewFailed.svelte';
  import { messages, t } from './lib/i18n/i18n.svelte.ts';
  import { Router, type Resolution } from './lib/router.ts';
  import { leaveGuard } from './lib/frame/leave.svelte.ts';
  import LeaveDialog from './lib/frame/LeaveDialog.svelte';
  import { loadReleaseAt } from './lib/frame/release.ts';
  import { PLACE_ROUTES, ROUTES, isSettingsRoute, movedTo, movedWith, paneFor } from './lib/routes.ts';
  import { viewport } from './lib/frame/viewport.svelte.ts';
  import { actor } from './lib/data/account.svelte.ts';
  import { manifest } from './lib/data/capabilities.svelte.ts';
  import ModuleOff from './lib/frame/ModuleOff.svelte';
  import { PRODUCT_HOMES, isProductHome, offModuleOf } from './lib/frame/moduleroute.ts';
  import { holdsHere, reachesAdministrationArea } from './lib/data/capability.svelte.ts';
  import { live } from './lib/data/live.svelte.ts';
  import { viewAs } from './lib/data/viewas.svelte.ts';
  import { platform } from './lib/platform/index.ts';
  import { session } from './lib/session.svelte.ts';
  import { precacheWhenIdle } from './lib/frame/precache.ts';
  import { loadFrame, partLoaders, ruleViews, workViews } from './lib/frame/parts.ts';
  import { onceEach } from './lib/frame/once.ts';
  import { connectorReturnOf } from './lib/data/connectorreturn.ts';
  import { engine } from './lib/data/engine.ts';
  import { forwardTo } from './lib/data/oidc.ts';
  import SignInCard from './lib/signin/SignInCard.svelte';
  import { deviceList } from './lib/data/devicelist.svelte.ts';
  import YourWorkspacesView from './views/YourWorkspacesView.svelte';
  import { appSignIn } from './lib/signin/appsignin.svelte.ts';
  import { appWorkspaces } from './lib/data/appworkspaces.svelte.ts';
  import OidcCallbackView from './views/OidcCallbackView.svelte';
  import RedeemView from './views/RedeemView.svelte';
  import ResetView from './views/ResetView.svelte';
  import ConfirmAddressView from './views/ConfirmAddressView.svelte';
  import RateView from './views/RateView.svelte';
  import SetupView from './views/SetupView.svelte';
  import SignInView from './views/SignInView.svelte';

  // The table lives in `lib/routes.ts` so that ADR-0032's areas can be asserted as a set:
  // a rule about which routes belong to the administration area can only be tested where the
  // set can be imported, and a table declared here could only be checked by mounting this.
  // Route-level code splitting (F-10): every screen but the doors before the frame is its own
  // chunk, loaded when its route is first reached; the frame and the work's own screens - today, a
  // place, an entry - are fetched together as soon as somebody is signed in.
  const views = onceEach({
    ...workViews,
    AgentsView: () => import('./views/AgentsView.svelte'),
    AgentsSettingsView: () => import('./views/AgentsSettingsView.svelte'),
    CmdbView: () => import('./views/CmdbView.svelte'),
    ServiceDeskView: () => import('./views/ServiceDeskView.svelte'),
    TimeSettingsView: () => import('./views/TimeSettingsView.svelte'),
    AgentView: () => import('./views/AgentView.svelte'),
    ArchiveView: () => import('./views/ArchiveView.svelte'),
    JumbleView: () => import('./views/JumbleView.svelte'),
    InstallationView: () => import('./views/InstallationView.svelte'),
    ProfileView: () => import('./views/ProfileView.svelte'),
    PlaceSettingsView: () => import('./views/PlaceSettingsView.svelte'),
    AppearanceView: () => import('./views/AppearanceView.svelte'),
    NotificationsView: () => import('./views/NotificationsView.svelte'),
    SecurityView: () => import('./views/SecurityView.svelte'),
    SessionsView: () => import('./views/SessionsView.svelte'),
    DevicesView: () => import('./views/DevicesView.svelte'),
    GrantsView: () => import('./views/GrantsView.svelte'),
    CalendarPushesView: () => import('./views/CalendarPushesView.svelte'),
    FileStoresView: () => import('./views/FileStoresView.svelte'),
    SearchView: () => import('./views/SearchView.svelte'),
    MyTokensView: () => import('./views/MyTokensView.svelte'),
    NewTokenView: () => import('./views/NewTokenView.svelte'),
    SecondFactorView: () => import('./views/SecondFactorView.svelte'),
    TokenView: () => import('./views/TokenView.svelte'),
    TrashView: () => import('./views/TrashView.svelte'),
    PagesView: () => import('./views/PagesView.svelte'),
    NotesView: () => import('./views/NotesView.svelte'),
    ProductHomeView: () => import('./views/ProductHomeView.svelte'),
    HubDeskView: () => import('./views/HubDeskView.svelte'),
    ApprovalsView: () => import('./views/ApprovalsView.svelte'),
    PortalView: () => import('./views/PortalView.svelte'),
    PortalRequestView: () => import('./views/PortalRequestView.svelte'),
    PortalArticleView: () => import('./views/PortalArticleView.svelte'),
    WhiteboardsView: () => import('./views/WhiteboardsView.svelte'),
    PlanView: () => import('./views/PlanView.svelte'),
    SprintView: () => import('./views/SprintView.svelte'),
    ReleaseView: () => import('./views/ReleaseView.svelte'),
    TimeView: () => import('./views/TimeView.svelte'),
    GoalsView: () => import('./views/GoalsView.svelte'),
    GoalView: () => import('./views/GoalView.svelte'),
    PortfolioView: () => import('./views/PortfolioView.svelte'),
    RoadmapView: () => import('./views/RoadmapView.svelte'),
    DashboardView: () => import('./views/DashboardView.svelte'),
    ActualsView: () => import('./views/ActualsView.svelte'),
    WorkloadView: () => import('./views/WorkloadView.svelte'),
    AssetsView: () => import('./views/AssetsView.svelte'),
    ...ruleViews,
    // Doors few first screens open - an app's consent, the installed app's own sign-in and its
    // passkey relay, a public form and roadmap - so the first screen's budget
    // (`build/budget.test.js`) carries none of them.
    PublicFormView: () => import('./views/PublicFormView.svelte'),
    PublicRoadmapView: () => import('./views/PublicRoadmapView.svelte'),
    ConsentView: () => import('./views/ConsentView.svelte'),
    AppSignInView: () => import('./lib/signin/AppSignInView.svelte'),
    KeyRelayView: () => import('./lib/signin/KeyRelayView.svelte'),
    // The two areas the Android build leaves to the web app, each one module (`platform/target.ts`).
    AdministrationArea: () => import('./views/AdministrationArea.svelte'),
    InstanceArea: () => import('./views/InstanceArea.svelte'),
  });

  // The agents' settings tab is a settings page of its own; `models` and `metering` were tabs once.
  // Whoever does not administer agents has none and stays on the list (P-05).
  const isAgentsSettings = (tab: string | undefined) =>
    (tab === 'settings' || tab === 'models' || tab === 'metering') && holdsHere('MANAGE_MEMBERS') === true;

  const router = new Router(ROUTES, undefined, { fullLoad: loadReleaseAt, admits: (path) => leaveGuard.admits(path) });
  $effect(() => leaveGuard.connect((path) => router.navigate(path)));
  let route = $state<Resolution>(router.current);

  /**
   * A mailbox's, calendar push's or file store's provider return, which in multi-tenant operation
   * lands on the installation's host while its session is on the workspace's (identity.md §10.1).
   * Asked once, at load, before the sign-in card or the page's own completion is drawn: either
   * would act on this host's session, or on none. Any failure finishes here, as before.
   */
  const connectorReturn = connectorReturnOf(location.pathname, location.search);
  let locating = $state(connectorReturn !== undefined);
  if (connectorReturn !== undefined) {
    void engine
      .mutate<ConnectorLocated>('POST', '/auth/connector:locate', connectorReturn)
      .then((answer) => forwardTo(answer.base_url, location, location.search, location.pathname))
      .catch(() => undefined)
      .then((target) => {
        if (target === undefined) locating = false;
        else location.replace(target);
      });
  }

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
    // Only on a measured width: before the frame has started the viewport, `isLarge` is its
    // default, and a desktop's `?item=` would be sent to the entry's page.
    if (pane.kind === 'redirect' && viewport.isMeasured) router.replace(pane.path);
  });
  // An address whose screen became a tab elsewhere (`movedTo`) replaces itself with that tab's,
  // its query kept.
  const moved = $derived(movedTo(route.name));
  $effect(() => {
    if (moved) router.replace(movedWith(moved, route.query));
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
  /** The module a page belongs to where it is off: said in place of the page (moduleroute.ts). */
  const offModule = $derived(offModuleOf(route.name ?? '', manifest.value?.modules_on));
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
   * After a sign-in in multi mode, the workspace's entry goes once to the installation's own
   * address, where the device keeps its list of workspaces, and the browser comes back here
   * (UC-ID-25 check 1, ADR-0095 §7). Once the account has arrived, so the path it comes back to is
   * the one the sign-in went on to, and once the manifest is read, which says whether there is a
   * list at all - in single mode the sign-in is taken and nothing happens (check 8).
   */
  $effect(() => {
    if (!session.isSignedIn || !session.isFreshSignIn || !actor.account || !manifest.isRead) return;
    session.takeFreshSignIn();
    void deviceList.rememberHere(location.pathname + location.search);
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
  // An installed app's sign-in, once the account has arrived: the workspace named, the toast said.
  let welcomed = false;
  $effect(() => {
    if (!session.isSignedIn || !actor.account || welcomed || platform.browserSignIn === undefined) return;
    welcomed = true;
    void appSignIn.welcome();
  });
  // And the grants a workspace forgotten offline still owes, withdrawn once per start.
  appWorkspaces.settle();

  const liveAccountId = $derived(
    // The account from `/accounts/me`, or the one remembered beside the pair when the server
    // cannot be reached: a tab reloading offline still opens its replica. Not while the session
    // shows the app as another person (UC-ID-29): a view is online only, and no copy is kept.
    session.isSignedIn && viewAs.settled && !viewAs.running
      ? (actor.account?.id ?? platform.lastAccount())
      : undefined,
  );
  $effect(() => {
    if (!session.isSignedIn) return;
    return untrack(() => viewAs.open());
  });
  // Every screen's and part's chunk once the signed-in session is idle, so one never opened before
  // going offline still opens from the copy (precache.ts).
  // Signed in: the frame and the work's screens are fetched at once and together, not one after
  // the other as each is drawn.
  $effect(() => {
    if (!session.isSignedIn) return;
    void loadFrame().catch(() => {});
    for (const load of Object.values(workViews)) void load().catch(() => {});
  });
  let precached = false;
  $effect(() => {
    if (!session.isSignedIn || precached) return;
    precached = true;
    void precacheWhenIdle([...Object.values(views), ...partLoaders()]);
  });
  $effect(() => {
    const accountId = liveAccountId;
    if (!accountId) return;
    live.start(accountId);
    return () => live.stop();
  });

  // A screen that throws while drawing is caught by the boundary inside the frame, so the frame and
  // its navigation stay usable; leaving the address draws the next screen afresh.
  let resetView: (() => void) | undefined;
  $effect(() => {
    void route.path;
    untrack(() => {
      resetView?.();
      resetView = undefined;
    });
  });
</script>

<!-- The outer boundary catches what the frame itself throws: the page still says what happened and
     offers a reload, instead of going blank. -->
<svelte:boundary>
<!-- Signed out there is no shell, and that is the decision rather than an omission (ADR-0061 draws
     the bar for a page that has navigation; this one has none). The card is the page: it brings
     its own `main`, its own canvas and the footer the operator's legal links live in, so a screen
     nobody can navigate away from is not wrapped in the furniture of navigating.
     The two link screens are here whether or not somebody is signed in: an invitation and a reset
     link are proof about an account that need not be the one in this tab, and a person who opened
     one from their mail while signed in would otherwise meet "there is nothing at this address". -->
{#if locating}
  <SignInCard title={t('app.callback.connect_title')}>
    <p><Spinner /> {t('app.callback.connect_working')}</p>
  </SignInCard>
{:else if route.name === 'oidc-callback'}
  <!-- The provider's return, on the card and never in the frame (UC-ID-08 check 5): this address
       is reached by a redirect into a fresh document, before there is a session, and the frame of
       an application nobody is signed into is furniture around one sentence. Whether or not a
       session exists: the moment the exchange opens one, this screen sends the reader on, and a
       second copy of it mounted inside the frame would find the address already cleaned and
       report a refusal for a sign-in that succeeded. -->
  <OidcCallbackView onnavigate={(path) => router.navigate(path)} />
{:else if route.name === 'public-form'}
  <!-- A public form, on the card whether or not somebody is signed in (UC-SVC-13): it files as
       the link's maker, never as the reader of this tab. -->
  <LazyView load={views.PublicFormView} />
{:else if route.name === 'public-roadmap'}
  <!-- A portfolio's roadmap opened by its public link (UC-GOL-03 check 15), whether or not
       somebody is signed in: it reads as the link's maker, never as this tab's reader. -->
  <LazyView load={views.PublicRoadmapView} />
{:else if route.name === 'rate'}
  <!-- A resolution mail's satisfaction link, on the card whether or not somebody is signed in
       (UC-SVC-07 check 6): it rates for the requester it was mailed to, never as this tab's reader. -->
  <RateView />
{:else if route.name === 'app-passkey'}
  <!-- An installed app's passkey step-up, in the system browser at the workspace's host
       (ADR-0096 decision 5). Whether or not this browser is signed in: the proof is for the app's
       session, and the answer goes back to the app. -->
  <LazyView load={views.KeyRelayView} />
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
  {:else if platform.browserSignIn}
    <!-- An installed app signs in through the system browser at the workspace's own host
         (ADR-0096), so its card asks which workspace and waits; the sign-in card is the browser's. -->
    <LazyView load={views.AppSignInView} />
  {:else if manifest.isSetupOpen}
    <!-- A fresh installation: nothing to sign in to yet, so the first start's setup takes the
         sign-in card's place until the first workspace exists (UC-INS-01). -->
    <SetupView onnavigate={(path) => router.navigate(path)} />
  {:else if deviceList.isHome}
    <!-- The installation's own address in multi mode, signed out: the device's list of workspaces
         and no sign-in card - no workspace answers here, and no field looks one up (UC-ID-25
         check 3, Decision 7). -->
    <YourWorkspacesView />
  {:else if deviceList.mayBeHome}
    <!-- A redirect to the list's home, before the manifest said whether this is it: nothing drawn,
         so the redirect there and back stays invisible. -->
  {:else}
    <SignInView />
  {/if}
{:else if route.name === 'consent'}
  <!-- Signed in only: `POST /oauth/authorize` needs a person, never a token, so somebody who
       arrives here signed out meets the sign-in screen first and lands back on this address. A
       card like the sign-in's rather than the frame: the question is the workspace's to the
       person, about an app, and nothing else on the page may be pressed instead (UC-SYN-07/3). -->
  <LazyView load={views.ConsentView} />
{:else}
<LazyFrame {route} onnavigate={(path) => router.navigate(path)}>
  <svelte:boundary onerror={(_, reset) => (resetView = reset)}>
  {#if reach === undefined}
    <!-- Not yet known whether this reader is inside the level: nothing, rather than a guess. -->
  {:else if reach === false}
    {@render nothingHere()}
  {:else if offModule}
    <ModuleOff module={offModule} isProduct={route.name !== null && isProductHome(route.name)} />
  {:else if route.name === 'home'}
    <LazyView load={views.TodayView} props={{ onnavigate: (path: string) => router.navigate(path) }} />
  {:else if route.name === 'installation'}
    <LazyView load={views.InstallationView} />
  {:else if route.name === 'profile' || route.name === 'profile-section'}
    <LazyView load={views.ProfileView} props={{ section: route.params.section, onreplace: (path: string) => router.replace(path) }} />
  {:else if route.name === 'tokens'}
    <LazyView load={views.MyTokensView} props={{ onnavigate: (path: string) => router.navigate(path) }} />
  {:else if route.name === 'token-new'}
    <LazyView load={views.NewTokenView} props={{ onnavigate: (path: string) => router.navigate(path) }} />
  {:else if route.name === 'token'}
    {#key route.params.id}
      <LazyView load={views.TokenView} props={{ id: route.params.id ?? '', onnavigate: (path: string) => router.navigate(path) }} />
    {/key}
  {:else if route.name === 'appearance'}
    <LazyView load={views.AppearanceView} props={{ onnavigate: (path: string) => router.navigate(path) }} />
  {:else if route.name === 'notifications'}
    <LazyView load={views.NotificationsView} props={{ onnavigate: (path: string) => router.navigate(path) }} />
  {:else if route.name === 'security' || route.name === 'security-section'}
    <LazyView load={views.SecurityView} props={{ section: route.params.section, onnavigate: (path: string) => router.navigate(path), onreplace: (path: string) => router.replace(path) }} />
  {:else if route.name === 'second-factor-new'}
    <LazyView load={views.SecondFactorView} props={{ onnavigate: (path: string) => router.navigate(path) }} />
  {:else if route.name === 'sessions'}
    <LazyView load={views.SessionsView} />
  {:else if route.name === 'devices'}
    <LazyView load={views.DevicesView} />
  {:else if route.name === 'grants'}
    <LazyView load={views.GrantsView} />
  {:else if route.name === 'calendars'}
    <LazyView load={views.CalendarPushesView} props={{ providerAnswer: route.query, onanswered: () => router.replace('/profile/calendars') }} />
  {:else if route.name === 'filestores'}
    <LazyView load={views.FileStoresView} props={{ providerAnswer: route.query, onanswered: () => router.replace('/profile/files') }} />
  {:else if route.name === 'own-rules'}
    <LazyView load={views.RulesView} props={{ isOwn: true, onnavigate: (path: string) => router.navigate(path) }} />
  {:else if route.name === 'own-rule-new'}
    <LazyView load={views.RuleNewView} props={{ isOwn: true, onnavigate: (path: string) => router.navigate(path) }} />
  {:else if route.name === 'own-rule'}
    {#key route.params.id}
      <LazyView load={views.RuleView} props={{ id: route.params.id ?? '', isOwn: true, onnavigate: (path: string) => router.navigate(path) }} />
    {/key}
  {:else if route.name === 'own-agents' && isAgentsSettings(route.query['tab'])}
    <LazyView load={views.AgentsSettingsView} props={{ tab: route.query['tab'], section: route.query['section'], base: '/profile/agents', onreplace: (path: string) => router.replace(path) }} />
  {:else if route.name === 'own-agents'}
    <LazyView load={views.AgentsView} props={{ tab: route.query['tab'], agent: route.query['agent'], base: '/profile/agents', onnavigate: (path: string) => router.replace(path) }} />
  {:else if route.name === 'own-agent'}
    {#key route.params.id}
      <LazyView load={views.AgentView} props={{ id: route.params.id ?? '', base: '/profile/agents', onnavigate: (path: string) => router.navigate(path) }} />
    {/key}
  {:else if movedTo(route.name)}
    <!-- Nothing for the moment before the address replaces itself, rather than a not-found. -->
  {:else if route.area === 'instance'}
    <!-- The level above the workspaces (ADR-0070 §5). Its own area, and its own door: every screen
         here is drawn only while the session is raised, and the hour's end returns the reader to
         the application rather than leaving them looking at numbers nothing will refresh. -->
    <LazyView load={views.InstanceArea} props={{ route, onnavigate: (path: string) => router.navigate(path), onreplace: (path: string) => router.replace(path) }} />
  {:else if route.area === 'administration'}
    <LazyView load={views.AdministrationArea} props={{ route, onnavigate: (path: string) => router.navigate(path), onreplace: (path: string) => router.replace(path) }} />
  {:else if route.name === 'search'}
    <LazyView load={views.SearchView} props={{ query: route.query, onnavigate: (path: string) => router.replace(path) }} />
  {:else if route.name === 'jumble'}
    <LazyView load={views.JumbleView} props={{ onnavigate: (path: string) => router.navigate(path) }} />
  {:else if route.name === 'trash'}
    <LazyView load={views.TrashView} props={{ kind: route.query['kind'], onnavigate: (path: string) => router.replace(path) }} />
  {:else if route.name === 'archive'}
    <LazyView load={views.ArchiveView} props={{ onnavigate: (path: string) => router.navigate(path) }} />
  {:else if route.name === 'item'}
    {#key route.params.id}
      <LazyView load={views.ItemView} props={{ id: route.params.id ?? '', onnavigate: (path: string) => router.navigate(path) }} />
    {/key}
  {:else if route.name === 'approvals'}
    <LazyView load={views.ApprovalsView} props={{ onnavigate: (path: string) => router.navigate(path) }} />
  {:else if route.name === 'notes'}
    <!-- *My notes* is the pages' home's second tab (user-experience.md §2.2), at its own address. -->
    <LazyView load={views.ProductHomeView} props={{ module: 'KNOWLEDGE', tab: 'notes', onnavigate: (path: string) => router.navigate(path) }} />
  {:else if route.name === 'assets-settings'}
    <!-- A product's settings are a settings page of their own, its sections in place of the home's
         tabs (page-system §9.6, PS-2). -->
    <LazyView load={views.CmdbView} props={{ section: route.query['section'], onreplace: (path: string) => router.replace(path) }} />
  {:else if route.name === 'desk-settings'}
    <LazyView load={views.ServiceDeskView} props={{ section: route.query['section'], onnavigate: (path: string) => router.navigate(path), onreplace: (path: string) => router.replace(path) }} />
  {:else if route.name === 'desk-desks'}
    <LazyView load={views.ProductHomeView} props={{ module: 'SERVICE_DESK', tab: 'desks', onnavigate: (path: string) => router.navigate(path) }} />
  {:else if route.name === 'agents-home' && isAgentsSettings(route.query['tab'])}
    <!-- The agents' settings are a settings page of their own (page-system §9.6). -->
    <LazyView load={views.AgentsSettingsView} props={{ tab: route.query['tab'], section: route.query['section'], base: '/agents', onreplace: (path: string) => router.replace(path) }} />
  {:else if route.name === 'agents-home'}
    <!-- The agents' home is their list (user-experience.md §7.8), at the product's address. -->
    <LazyView load={views.AgentsView} props={{ tab: route.query['tab'], agent: route.query['agent'], base: '/agents', onnavigate: (path: string) => router.navigate(path) }} />
  {:else if route.name === 'agent'}
    {#key route.params.id}
      <LazyView load={views.AgentView} props={{ id: route.params.id ?? '', base: '/agents', onnavigate: (path: string) => router.navigate(path) }} />
    {/key}
  {:else if route.name && PRODUCT_HOMES[route.name]}
    <LazyView load={views.ProductHomeView} props={{ module: PRODUCT_HOMES[route.name], onnavigate: (path: string) => router.navigate(path) }} />
  {:else if route.name === 'portal'}
    {#key route.path}
      <LazyView load={views.PortalView} props={{ collectionId: route.params.id ?? '', onnavigate: (path: string) => router.navigate(path) }} />
    {/key}
  {:else if route.name === 'portal-request'}
    {#key route.path}
      <LazyView load={views.PortalRequestView} props={{ id: route.params.id ?? '', onnavigate: (path: string) => router.navigate(path) }} />
    {/key}
  {:else if route.name === 'portal-article'}
    {#key route.path}
      <LazyView load={views.PortalArticleView} props={{ collectionId: route.params.id ?? '', pageId: route.params.pageId ?? '', onnavigate: (path: string) => router.navigate(path) }} />
    {/key}
  {:else if route.name === 'hub-pages' || route.name === 'page'}
    {#key route.path}
      <LazyView
        load={views.PagesView}
        props={{
          hubId: route.name === 'hub-pages' ? route.params.id : undefined,
          pageId: route.name === 'page' ? route.params.id : undefined,
          isCreateAsked: route.query['new'] === '1',
          onnavigate: (path: string) => router.navigate(path),
          onreplace: (path: string) => router.replace(path),
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
          isCreateAsked: route.query['new'] === '1',
          onnavigate: (path: string) => router.navigate(path),
        }}
      />
    {/key}
  {:else if route.name === 'hub-desk'}
    {#key route.path}
      <LazyView load={views.HubDeskView} props={{ hubId: route.params.id ?? '', onnavigate: (path: string) => router.navigate(path) }} />
    {/key}
  {:else if route.name === 'hub-plan' || route.name === 'collection-plan'}
    {#key route.path}
      <LazyView load={views.PlanView} props={{ planId: route.params.id, view: route.query['view'], onnavigate: (path: string) => router.navigate(path), onreplace: (path: string) => router.replace(path) }} />
    {/key}
  {:else if route.name === 'time' && route.query['tab'] === 'settings'}
    <LazyView load={views.TimeSettingsView} props={{ section: route.query['section'], onreplace: (path: string) => router.replace(path) }} />
  {:else if route.name === 'time'}
    <LazyView load={views.TimeView} props={{ tab: route.query['tab'], week: route.query['week'], onnavigate: (path: string) => router.replace(path) }} />
  {:else if route.name === 'goals'}
    <LazyView load={views.GoalsView} props={{ tab: route.query['tab'], period: route.query['period'], hub: route.query['hub'], onnavigate: (path: string) => router.replace(path) }} />
  {:else if route.name === 'goal'}
    {#key route.path}
      <LazyView load={views.GoalView} props={{ goalId: route.params.id, onnavigate: (path: string) => router.navigate(path) }} />
    {/key}
  {:else if route.name === 'portfolio'}
    {#key route.path}
      <LazyView load={views.PortfolioView} props={{ portfolioId: route.params.id, onnavigate: (path: string) => router.navigate(path) }} />
    {/key}
  {:else if route.name === 'portfolio-roadmap'}
    {#key route.path}
      <LazyView load={views.RoadmapView} props={{ portfolioId: route.params.id, onnavigate: (path: string) => router.navigate(path) }} />
    {/key}
  {:else if route.name === 'dashboard'}
    {#key route.path}
      <LazyView load={views.DashboardView} props={{ dashboardId: route.params.id, onnavigate: (path: string) => router.navigate(path) }} />
    {/key}
  {:else if route.name === 'hub-actuals' || route.name === 'collection-actuals'}
    {#key route.path}
      <LazyView load={views.ActualsView} props={{ planId: route.params.id, onnavigate: (path: string) => router.navigate(path) }} />
    {/key}
  {:else if route.name === 'hub-workload' || route.name === 'collection-workload'}
    {#key route.path}
      <LazyView load={views.WorkloadView} props={{ planId: route.params.id, onnavigate: (path: string) => router.navigate(path) }} />
    {/key}
  {:else if route.name === 'hub-releases'}
    {#key route.path}
      <LazyView load={views.ReleaseView} props={{ hubId: route.params.id, onnavigate: (path: string) => router.navigate(path) }} />
    {/key}
  {:else if route.name === 'collection-sprints'}
    {#key route.path}
      <LazyView load={views.SprintView} props={{ collectionId: route.params.id, isCreateAsked: route.query['new'] === '1', onnavigate: (path: string) => router.navigate(path) }} />
    {/key}
  {:else if route.name === 'hub-assets' || route.name === 'hub-assets-section' || route.name === 'ci'}
    {#key route.name === 'ci' ? route.path : route.params.id}
      <LazyView
        load={views.AssetsView}
        props={{
          hubId: route.name === 'ci' ? undefined : route.params.id,
          section: route.name === 'hub-assets-section' ? route.params.section : undefined,
          ciId: route.name === 'ci' ? route.params.id : undefined,
          isCreateAsked: route.query['new'] === '1',
          onnavigate: (path: string) => router.navigate(path),
        }}
      />
    {/key}
  {:else if PLACE_ROUTES.includes(route.name ?? '') && isSettingsRoute(route.name)}
    <!-- A place's settings are their own page kind (page-system §9.6); keyed on the id, as the place is. -->
    {#key route.params.id}
      <LazyView load={views.PlaceSettingsView} props={{
        id: route.params.id ?? '',
        section: route.params.section,
        template: route.query['template'],
        onnavigate: (path: string) => router.navigate(path),
        onreplace: (path: string) => router.replace(path),
      }} />
    {/key}
  {:else if PLACE_ROUTES.includes(route.name ?? '')}
    <!-- One view for both: they differ in what they hold, not in what they are. Keyed on the id so
         that navigating from one collection to another rebuilds rather than reusing the state of
         the one before — a draft rename would otherwise follow the reader to a different name. -->
    {#key route.params.id}
      <LazyView load={views.ContainerView} props={{
        id: route.params.id ?? '',
        openItemId: pane.kind === 'pane' ? pane.itemId : undefined,
        onnavigate: (path: string) => router.navigate(path),
      }} />
    {/key}
  {:else}
    {@render nothingHere()}
  {/if}
  {#snippet failed(error, reset)}
    <ViewFailed {error} {reset} />
  {/snippet}
  </svelte:boundary>
</LazyFrame>
<LeaveDialog />
{/if}
{#snippet failed(error)}
  <main><ViewFailed {error} /></main>
{/snippet}
</svelte:boundary>

{#snippet nothingHere()}
  <!-- The server's own code for a path that reaches nothing, at the same address - as the
       page's heading, because a screen without one is a screen a reader cannot name. -->
  <h1>{t('route.unknown')}</h1>
  <p><code>{route.path}</code></p>
  <p><a href="/">{t('app.back_to_start')}</a></p>
{/snippet}
