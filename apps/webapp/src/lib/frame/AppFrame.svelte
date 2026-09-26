<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What every view sits inside: the shell wave drawn from the five widths (ADR-0061), the marks
  // the application owes the reader about itself, and the region the view is rendered into.
  //
  // One navigation, three drawings. `lib/navigation.ts` is the list; this frame draws it as a
  // `SideNav` pinned beside the content from `expanded` (collapsible to a rail), as the same
  // `SideNav` in a `NavDrawer` behind ☰ below it, and as a `BottomBar` on `compact`, where the
  // drawer then holds the tree alone. The account group is behind the avatar and the name from
  // `medium` and behind "You" in the bottom bar below it. The bar carries no page action. It does
  // carry the entry to search from `medium` up (ADR-0063 decision 4), and that is why `primary()`
  // is asked for the list *without* Search there: one visible entry to it on every width - the
  // field up here, or the destination in the bottom bar on `compact`, never both.
  //
  // Two things it deliberately does not do. It knows nothing about a Tauri shell — every platform
  // difference goes through `src/lib/platform/` (ADR-0033), and there is no `isTauri` anywhere in
  // this tree. And it holds no data of its own: the manifest is read once in
  // `lib/data/capabilities.svelte.ts`, and a view reads what it needs through the engine.

  import type { Snippet } from 'svelte';
  import { untrack } from 'svelte';

  import { AppBar, BottomBar, IconButton, Menu, NavDrawer, VisuallyHidden } from '@hubtask/design-system/components';

  import AccountMenu from './AccountMenu.svelte';
  import SectionNav from './SectionNav.svelte';
  import BarSearch from './BarSearch.svelte';
  import NoticeMark from './NoticeMark.svelte';
  import StepUpPrompt from './StepUpPrompt.svelte';
  import TourGuide from './TourGuide.svelte';
  import SyncLine from './SyncLine.svelte';
  import WorkspaceNav from './WorkspaceNav.svelte';
  import { page } from './page.svelte.ts';
  import { viewport } from './viewport.svelte.ts';

  import { announcer } from '../announce.svelte.ts';
  import { live } from '../data/live.svelte.ts';
  import { actor } from '../data/account.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { recents } from '../recents.svelte.ts';
  import { session } from '../session.svelte.ts';
  import { tour } from '../tour.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { quotas } from '../data/quotas.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { ADMINISTRATION, DESTINATIONS, SETTINGS, YOU_CODE, account, currentDestination, primary } from '../navigation.ts';
  import type { Resolution } from '../router.ts';

  interface Props {
    route: Resolution;
    /** Where the sidebar sends the reader. The frame does not own the router.  */
    onnavigate: (path: string) => void;
    children: Snippet;
  }

  const { route, onnavigate, children }: Props = $props();

  /** The landmark the skip link lands on. */
  let mainElement = $state<HTMLElement | null>(null);

  // Who is signed in, read once the frame is up - and read again whenever that changes, which is
  // what the `session.status` below is doing in an effect that otherwise depends on nothing. A
  // subscription taken before a sign-in would be a subscription the sign-out already dropped.
  $effect(() => {
    void session.status;
    return actor.start();
  });

  // The workspace's structure, read once for the whole application rather than per view: the
  // sidebar, the breadcrumb and a hub's list of collections are three readers of one page, and
  // three subscriptions to it would be three requests for the same answer. Started with the
  // session for the reason the account is — there is nothing to read without a bearer.
  $effect(() => {
    if (!session.isSignedIn) return;
    return containers.start();
  });

  /**
   * The one place the language is decided, because it is the one place that knows both halves:
   * what the reader prefers (their account, then their browser) and what the installation has
   * (the manifest). `i18n-l10n.md` §2's order, with the parenthesis that inverts its top - the
   * account wins over `Accept-Language`, which is what answers before there is an account.
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
   * The administration area, offered only where the server says this reader reaches it.
   *
   * **The one place in this client where hiding beats a `CapabilityGate`.** The gate exists so
   * that a control somebody might want carries its reason (`domain-model.md` §2); a whole area
   * they will never hold is not a control they want, it is navigation noise on every screen.
   *
   * The condition is the server's own: `GET /quotas` is refused unless the caller holds
   * `STRUCTURE` or the auditor's `READ_CONFIGURATION`, which is the area's condition exactly. A
   * frame that worked it out from the actor's role and the role matrix would be a second
   * implementation of an authorisation decision — and would be wrong for an auditor, who holds no
   * `READ` and therefore cannot list the memberships that would say what role they have.
   */
  $effect(() => {
    if (!session.isSignedIn) return;
    return untrack(() => quotas.open());
  });

  // Whose list of recently opened entries this device is holding. The account's id is the key, so
  // it is adopted when the account arrives and dropped when it changes — two people sharing a
  // browser never read each other's (`lib/recents.svelte.ts`).
  $effect(() => recents.adopt(actor.account?.id));

  // Which of the five widths the frame is drawn at. Started with the frame and stopped with it.
  $effect(() => viewport.start());

  /** The destination the route belongs to; what the bottom bar and the tree announce as current. */
  const destination = $derived(currentDestination(route));
  /**
   * The node the tree marks: the container the reader is in, or the destination they are on. On
   * the workspace page the workspace row; inside a collection the collection, not the row above it.
   */
  const currentNode = $derived(
    route.name === 'hub' || route.name === 'collection' ? route.params.id : destination,
  );
  /**
   * The section the reader is inside, or nothing while they are in the workspace.
   *
   * Two of them: the administration (ADR-0063 decision 7) and Your settings (ADR-0065 decision 3).
   * A section has a navigation of its own and the workspace's tree is not drawn beside it - the
   * reader is in a place, not in a corner of the workspace. The route's **area** answers which,
   * which is the same answer `currentDestination` gives the account group: one fact, read once.
   */
  const section = $derived(
    route.area === 'administration'
      ? { label: t('app.admin.nav'), groups: ADMINISTRATION }
      : route.area === 'profile'
        ? { label: t('app.nav.profile'), groups: SETTINGS }
        : undefined,
  );
  /** Which row of the section's list is current, by the id that list gives it. */
  const sectionRow = $derived(
    section?.groups.flatMap((group) => group.rows).find((row) => row.routes.includes(route.name ?? ''))?.id,
  );
  const accountGroup = $derived(account({ isAdministrationReachable: quotas.isReachable === true }));
  /** The bottom bar: the primary group and "You", the account group's head on a phone. */
  const bottomDestinations = $derived([
    ...primary().map((each) => ({
      id: each.id,
      label: t(each.code),
      icon: each.icon,
      href: each.target.kind === 'route' ? each.target.path : '/',
    })),
    { id: 'you', label: t(YOU_CODE), icon: 'user' as const, href: '/profile' },
  ]);
  const bottomCurrent = $derived(
    accountGroup.some((each) => each.id === destination) ? 'you' : destination === 'trash' ? 'workspace' : destination,
  );

  /** The drawer below `expanded`, closed by a navigation; focus returns to ☰. */
  let isDrawerOpen = $state(false);
  /** The account sheet on `compact`, opened by "You" in the bottom bar. */
  let isAccountOpen = $state(false);

  /**
   * The pinned navigation folded to its marks, from `expanded` up. A device convenience like the
   * theme (ADR-0043): kept in this browser, never in the account, because how much of the screen
   * a navigation may have is a question about this screen.
   */
  const RAIL_KEY = 'hubtask.nav';
  let isRail = $state(false);
  $effect(() => {
    try {
      isRail = localStorage.getItem(RAIL_KEY) === 'rail';
    } catch {
      isRail = false;
    }
  });
  function toggleRail() {
    isRail = !isRail;
    try {
      localStorage.setItem(RAIL_KEY, isRail ? 'rail' : 'pinned');
    } catch {
      // A browser that refuses storage still gets the fold, for as long as the page is open.
    }
  }

  function go(path: string) {
    isDrawerOpen = false;
    onnavigate(path);
  }

  /** What choosing a row of the account group does: the frame's verbs, and nothing of its own. */
  function chooseAccount(id: string) {
    const chosen = DESTINATIONS.find((each) => each.id === id);
    if (!chosen) return;
    if (chosen.target.kind === 'route') go(chosen.target.path);
    else if (chosen.target.action === 'tour') void tour.restart();
    else void session.signOut();
  }

  /**
   * The two transitions worth hearing, announced through the region that already exists.
   *
   * Only on a change, and only these two: a badge that narrated every reconnection attempt would
   * be a screen reader reading a heartbeat. Becoming live is worth saying because it changes what
   * the reader can trust; losing it is worth saying for the same reason.
   */
  let announced = $state<string | undefined>(undefined);
  $effect(() => {
    const state = live.state;
    if (!session.isSignedIn || state === announced) return;
    if (announced !== undefined) {
      if (state === 'live') announcer.say(t('app.live.became_live'));
      // Losing it is worth saying whichever half lost it: the stream cannot be reached, or the
      // device says it has no network at all.
      else if (state === 'reconnecting') announcer.say(t('app.live.lost'));
      else if (state === 'offline') announcer.say(t('app.live.device_offline'));
    }
    announced = state;
  });

  /**
   * How many recovery codes are left, said once, on the other side of a sign-in that spent one.
   *
   * `SessionTokens.recovery_codes_remaining` has carried this since H-02, with "zero is the number
   * to act on" written beside it in the contract, and no client had ever read it - so somebody
   * could spend their tenth code and learn nothing. It belongs to the frame rather than to the
   * sign-in screen because it is true *after* the sign-in, when that screen is gone.
   *
   * Said rather than drawn, and that is ADR-0065 decision 4 holding: a banner here would be a
   * statement about *this person's* sign-in in the row the shell keeps for statements about the
   * application. What is worth acting on outlives the announcement anyway - the count stands on
   * the security screen, beside the button that makes new ones.
   *
   * Taken rather than read: an announcement that repeated on every navigation would be a screen
   * reader saying it on every click.
   */
  $effect(() => {
    if (!session.isSignedIn) return;
    const left = session.takeRecoveryLeft();
    if (left === undefined) return;
    announcer.say(
      left === 0 ? t('app.sign_in.recovery_none') : t('app.sign_in.recovery_left', { count: String(left) }),
    );
  });

</script>

<div class="frame" data-density={viewport.isSpacious ? 'spacious' : undefined} data-filled={page.fills ? '' : undefined} data-bottombar={session.isSignedIn ? '' : undefined}>
  <!-- The first stop on every page (2.4.1). The keyboard walk of F5-11 counted eleven stops from
       the top of the frame to the first control of the content; this is the one that skips them.
       Hidden until it takes focus, so nobody with a pointer ever sees it. The click is handled
       here rather than left to the anchor: the router takes every same-origin link and would
       navigate to the same path with the fragment dropped. -->
  <VisuallyHidden as="div" isFocusable>
    <a
      class="skip"
      href="#main"
      onclick={(event) => {
        event.preventDefault();
        mainElement?.focus();
      }}
    >
      {t('app.skip_to_content')}
    </a>
  </VisuallyHidden>

  <!-- Signed out there is nowhere to go but the token screen, so the bar offers nothing that
       would land there under another name: no toggle, no account, the wordmark alone. -->
  <!-- On a phone the bar carries the page's title where the page told the frame one
       (`page.svelte.ts`); the head then reads its heading rather than drawing it. -->
  <AppBar
    label={t('app.nav.bar')}
    title={viewport.isCompact ? page.title : undefined}
    toggle={!session.isSignedIn
      ? undefined
      : viewport.isBelowExpanded
        ? { kind: 'drawer', label: isDrawerOpen ? t('app.nav.close') : t('app.nav.open'), isExpanded: isDrawerOpen, onToggle: () => (isDrawerOpen = !isDrawerOpen), tour: 'hubs' }
        : { kind: 'rail', label: isRail ? t('app.nav.expand') : t('app.nav.collapse'), isExpanded: !isRail, onToggle: toggleRail }}
  >
    {#snippet brand()}
      <!-- A name rather than a message: the product is called Hubtask in every language. -->
      <a class="wordmark" href="/">Hubtask</a>
    {/snippet}
    {#snippet search()}
      <!-- Only where there is room for it. On `compact` the bar is a title and two controls, and
           the reader reaches search through the bottom bar instead. -->
      {#if session.isSignedIn && !viewport.isCompact}
        <BarSearch onnavigate={go} />
      {/if}
    {/snippet}
    {#snippet menu()}
      <!-- The page's own menu, at the end of the compact bar (ADR-0061 decision 1's table). The
           list is the head's own folded one, handed over rather than assembled here: two foldings
           would eventually disagree about which action is an action and which is an item. -->
      {#if viewport.isCompact && page.menu}
        {@const offered = page.menu}
        <Menu label={offered.label} items={offered.items} placement={{ side: 'block-end', align: 'end' }} onselect={offered.onselect}>
          {#snippet trigger(props)}
            <IconButton icon="ellipsis" label={offered.label} data-opener={offered.opener} {...props} />
          {/snippet}
        </Menu>
      {/if}
    {/snippet}
    {#snippet end()}
      <!-- The copy and the server, as one mark (ADR-0063 decision 5): connected, reconnecting or
           offline; what waits to be sent and what the server refused; when the copy last
           synchronised. It was a line of every page that read "Connected" at its quietest; now the
           ordinary case says nothing until it is pressed, and what it said is behind it, whole. -->
      <!-- What the application has to say about itself - the stage, and the health report where
           the reader may read one and it says something is wrong (ADR-0065 decision 4). It was two
           banners above the head of every page; now it is a mark that says nothing until it is
           pressed, and nothing at all when there is nothing to say. -->
      <NoticeMark />
      <SyncLine />
      <!-- Drawn as soon as there is a session, not once the account has arrived: signing out has
           to be reachable while the server is away, and the name is "You" until it is known. -->
      {#if session.isSignedIn && !viewport.isCompact}
        <AccountMenu destinations={accountGroup} name={actor.account?.display_name ?? t('app.nav.you')} email={actor.account?.email} isSheet={false} hasName={viewport.isLarge} onchoose={chooseAccount} />
      {/if}
    {/snippet}
  </AppBar>

  <div class="body">
    <!-- The navigation is the frame's, not a view's: it is the same tree on every screen, and a
         view that rendered it would rebuild it on every navigation. Pinned from `expanded`, where
         the tour's `hubs` step finds it; below that the same tree is in the drawer and the step
         finds the bar's ☰ instead. -->
    {#if session.isSignedIn}
      {#if viewport.isBelowExpanded}
        <NavDrawer bind:isOpen={isDrawerOpen} title={section?.label ?? t('app.nav.title')} dismissLabel={t('app.nav.close')}>
          {#if section}
            <SectionNav label={section.label} groups={section.groups} current={sectionRow} onnavigate={go} />
          {:else}
            <WorkspaceNav current={currentNode} hasDestinations={!viewport.isCompact} onnavigate={go} />
          {/if}
        </NavDrawer>
      {:else}
        <aside class="sidenav" data-rail={isRail ? '' : undefined} data-tour={section ? undefined : 'hubs'}>
          {#if section}
            <!-- The section's own list, in place of the tree (ADR-0063 decision 7). The tour's
                 `hubs` step points at the tree, so it does not point here: a step that pointed at
                 a column the tree is not in would explain the wrong thing. -->
            <SectionNav label={section.label} groups={section.groups} current={sectionRow} {isRail} onnavigate={go} />
          {:else}
            <WorkspaceNav current={currentNode} {isRail} onnavigate={go} />
          {/if}
        </aside>
      {/if}
    {/if}
    <!-- `tabindex="-1"` so that the skip link has somewhere to land; a landmark is not a control,
         so it draws no ring when it does. -->
    <main id="main" tabindex="-1" data-filled={page.fills ? '' : undefined} bind:this={mainElement}>
      <div class="content">
        {@render children()}
      </div>
    </main>
  </div>

  <!-- The way to the accessibility statement on the screens that have no other one
       (design-system.md §10): signed out there is no account group and no "About Hubtask", so the
       foot of the sign-in screen is the only place it can be - and it is the place it matters
       most, because a barrier at the door is the one nothing behind the door makes up for. Signed
       in the statement is on `/installation` beside the versions, and this footer is gone: a
       landmark carrying one external link took 51 px off every screen, and off the canvas of a
       board, for a link nobody follows while they are working.

       The statement lives on the website, unversioned, so that what it says about a walk is not
       tied to the build that shipped before the walk. A top-level navigation, which `connect-src`
       does not govern; a new tab, because the application is what the reader was in the middle
       of. -->
  {#if !session.isSignedIn}
    <footer class="foot">
      <a href="https://hubtask.eu/accessibility/" target="_blank" rel="noopener">
        {t('app.about.accessibility')}
      </a>
    </footer>
  {/if}

  <!-- The primary destinations where a thumb is, on `compact` only; "You" opens the account group
       as a sheet from the bottom, because there is no avatar in the bar on a phone. -->
  {#if session.isSignedIn && viewport.isCompact}
    <BottomBar
      label={t('app.nav.label')}
      destinations={bottomDestinations}
      current={bottomCurrent}
      onnavigate={(id) => {
        if (id === 'you') isAccountOpen = true;
        else go(bottomDestinations.find((each) => each.id === id)?.href ?? '/');
      }}
    />
    <AccountMenu destinations={accountGroup} name={actor.account?.display_name ?? t('app.nav.you')} email={actor.account?.email} isSheet bind:isSheetOpen={isAccountOpen} onchoose={chooseAccount} />
  {/if}

  <!-- The proof a privileged action demands, rendered once. Any request may meet the refusal, so
       the prompt belongs to the frame rather than to whichever screen made the request (H-03). -->
  <StepUpPrompt />
  <!-- The tour (F6-14): walks routes, so it lives where every route is. -->
  {#if session.isSignedIn}
    <TourGuide {onnavigate} />
  {/if}

  <!-- The application's one live region, and it is in the frame because two of them compete: a
       screen reader watches both and reads whichever changed, in an order nobody chose. It is here
       before it has anything to say, which is the other half — a region created at the moment it
       has something to announce is a region nothing was watching. -->
  <p class="announcement" role="status" aria-live="polite">{announcer.message}</p>
</div>

<style>
  .frame {
    display: flex;
    flex-direction: column;
    min-height: 100vh;
    background: var(--bg-canvas);
    color: var(--text-primary);
  }

  .wordmark {
    font-family: var(--font-display);
    font-size: var(--fs-300);
    font-weight: var(--fw-semibold);
    color: var(--text-primary);
    text-decoration: none;
  }

  /* Rule 5, on every focusable thing in the frame. */
  a:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
    border-radius: var(--r-xs);
  }

  .body { display: flex; flex: 1; min-width: 0; }


  /* The pinned navigation, from `expanded`: as wide as the token says, and it stays in view while
     the page scrolls under the bar. Its own scroll, so a long tree does not lengthen the page. */
  .sidenav {
    /* The frame is its own plane (ADR-0063 decision 3): the bar and the bottom bar already take
       the surface, and a navigation on the content's canvas read as an indented part of the page
       rather than as the frame around it. */
    background: var(--bg-surface);
    /* A column, so that the navigation's own foot band reaches the bottom of it. */
    display: flex;
    flex-direction: column;
    position: sticky;
    inset-block-start: var(--layout-appbar-height);
    flex: none;
    /* The token is the whole column, padding included: at exactly `expanded` the content beside
       it has to be `medium` wide, or the page head folds as if on a phone. */
    box-sizing: border-box;
    inline-size: var(--layout-sidenav-width);
    max-block-size: calc(100vh - var(--layout-appbar-height));
    min-width: 0;
    padding: var(--sp-200) var(--sp-150);
    overflow: auto;
    border-inline-end: var(--bw-hairline) solid var(--border-subtle);
  }

  /* Folded to its marks: the same tree, clipped to the rail's width. Nothing is removed and the
     keyboard reaches every row; only the words wait for the fold to open. */
  .sidenav[data-rail] {
    inline-size: var(--layout-sidenav-rail);
    padding-inline: var(--sp-050);
    overflow-x: hidden;
  }

  main { flex: 1; min-width: 0; padding: var(--sp-300); }

  /* A page that draws its own edges takes the region whole: no padding around it and no reading
     measure, because a canvas is not a document (`page.svelte.ts`). */
  main[data-filled] { padding: 0; }

  main[data-filled] .content { max-inline-size: none; }

  /* From `expanded`, where a filled page keeps its own panel beside its canvas, the region is a
     **height** as well as a width (`milestone-F8.md` decision 31): the page is one screen and
     what scrolls is inside it, so the frame grows no second scrollbar underneath and a panel as
     tall as the region really ends where the region does - the rule editor's ended below the
     window by the height of everything above it. `min-block-size: 0` down the flex chain, or the
     automatic minimum size lets the region grow with its content anyway. Below that the page
     scrolls as every page does: the details are a sheet there, and a bar that scrolls away
     leaves the canvas the screen. */
  /* design-system-lint-ignore: `primitive.breakpoint.expanded` (905px); a media query cannot read a custom property. */
  @media (width >= 905px) {
    .frame[data-filled] { box-sizing: border-box; block-size: 100dvh; min-block-size: 0; }

    .frame[data-filled] .body { min-block-size: 0; }

    main[data-filled] { min-block-size: 0; overflow: hidden; }

    main[data-filled] .content { block-size: 100%; }
  }

  main:focus { outline: none; }

  /* Above `xlarge` the content is capped and centred rather than stretched across the screen;
     the cap is the token's and applies from wherever the screen is wider than it. */
  .content { max-inline-size: var(--layout-content-max); margin-inline: auto; }

  .foot {
    padding: var(--sp-200) var(--sp-300);
    border-block-start: var(--bw-hairline) solid var(--border-subtle);
    font-size: var(--fs-075);
    color: var(--text-subtle);
  }

  .foot a { color: inherit; }

  .foot a:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  /* Below `medium` the bottom bar is fixed over the end of the page, so the page ends above it -
     the last row of a list is reachable rather than under the bar. Only where there is a bar:
     the bar is drawn for a session, and a frame that reserved its height signed out ended the
     sign-in screen 56 px above the bottom of the screen with nothing in the gap. The token's
     value is written out because a media query cannot read a custom property; it is
     `primitive.breakpoint.medium` and nothing else, and the token remains the source. */
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    .frame[data-bottombar] { padding-block-end: calc(var(--layout-bottombar-height) + env(safe-area-inset-bottom, 0)); }

    main { padding: var(--sp-200); }
  }

  .skip {
    display: inline-block;
    padding: var(--sp-100) var(--sp-200);
    border-radius: var(--r-md);
    background: var(--bg-surface);
    color: var(--text-primary);
    font-weight: var(--fw-medium);
  }

  .skip:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  /* Announced and not drawn. What it says is already on the screen — the row is where it now is —
     so printing it as well would be noise for every reader who can see the list. */
  .announcement {
    position: absolute;
    inline-size: var(--sp-025);
    block-size: var(--sp-025);
    margin: calc(var(--sp-025) * -1);
    padding: 0;
    overflow: hidden;
    clip-path: inset(50%);
    white-space: nowrap;
    border: 0;
  }
</style>
