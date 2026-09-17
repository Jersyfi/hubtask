<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What every view sits inside: the header with the navigation, the notices the application owes
  // the reader about itself, and the region the view is rendered into.
  //
  // Two things it deliberately does not do. It knows nothing about a Tauri shell — every platform
  // difference goes through `src/lib/platform/` (ADR-0033), and there is no `isTauri` anywhere in
  // this tree. And it holds no data of its own: the manifest is read once in
  // `lib/data/capabilities.svelte.ts`, and a view reads what it needs through the engine.

  import type { Snippet } from 'svelte';
  import { untrack } from 'svelte';

  import { Banner, Button, Inline, Menu, Stack, VisuallyHidden } from '@hubtask/design-system/components';

  import HealthNotice from './HealthNotice.svelte';
  import StepUpPrompt from './StepUpPrompt.svelte';
  import TourGuide from './TourGuide.svelte';
  import SyncLine from './SyncLine.svelte';
  import WorkspaceNav from './WorkspaceNav.svelte';

  import { announcer } from '../announce.svelte.ts';
  import { live } from '../data/live.svelte.ts';
  import { actor } from '../data/account.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { session } from '../session.svelte.ts';
  import { tour } from '../tour.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { quotas } from '../data/quotas.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { MATURITY, shouldAnnounce } from '../maturity.ts';
  import type { Resolution } from '../router.ts';

  interface Props {
    route: Resolution;
    /** Where the sidebar sends the reader. The frame does not own the router.  */
    onnavigate: (path: string) => void;
    children: Snippet;
  }

  const { route, onnavigate, children }: Props = $props();

  // Dismissed for as long as this page is open, and no longer. ADR-0035 §2 asks for a banner that
  // is not in the way; it does not ask the client to remember a decision across visits, and a
  // client that did would need somewhere to keep it - which is the platform seam's question and
  // F6's storage port, not this component's.
  let dismissed = $state(false);
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

  const links = $derived([
    { path: '/', name: 'home', label: 'app.nav.home' },
    { path: '/search', name: 'search', label: 'app.nav.search' },
    { path: '/jumble', name: 'jumble', label: 'app.nav.jumble' },
    { path: '/trash', name: 'trash', label: 'app.nav.trash' },
    { path: '/installation', name: 'installation', label: 'app.nav.installation' },
    ...(quotas.isReachable === true
      ? [{ path: '/administration', name: 'administration', label: 'app.nav.administration' }]
      : []),
    // Reachable from every screen, because it is where somebody goes when the product is speaking
    // to them in the wrong language — which is exactly the moment a buried link is no use.
    { path: '/profile', name: 'profile', label: 'app.nav.profile' },
  ]);
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
      else if (state === 'reconnecting') announcer.say(t('app.live.lost'));
    }
    announced = state;
  });

</script>

<div class="frame">
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
  <header class="bar">
    <!-- A name rather than a message: the product is called Hubtask in every language. -->
    <a class="wordmark" href="/">Hubtask</a>
    <!-- Signed out there is nowhere to go but the token screen, so the frame offers nothing that
         would land there under another name. -->
    {#if session.isSignedIn}
      <!-- Named for what it is, not after its first link: a landmark called "Home" is a landmark
           a reader of landmarks cannot tell from the link (F5-13). -->
      <nav aria-label={t('app.nav.label')}>
        <ul>
          {#each links as link (link.path)}
            <li>
              <a href={link.path} aria-current={route.name === link.name ? 'page' : undefined}>
                {t(link.label)}
              </a>
            </li>
          {/each}
        </ul>
      </nav>
      <div class="actor">
        <Inline gap="150" align="center">
          {#if actor.account}
            <span class="who">{t('app.signed_in_as', { name: actor.account.display_name })}</span>
          {/if}
          <!-- The help menu (§8): where the tour is taken again. One entry today, a menu rather
               than a button so that the second entry has somewhere to go. -->
          <Menu
            label={t('app.help.label')}
            items={[{ id: 'tour', label: t('app.help.tour_again'), icon: 'info' }]}
            placement={{ side: 'block-end', align: 'end' }}
            onselect={(id) => {
              if (id === 'tour') void tour.restart();
            }}
          >
            {#snippet trigger(props)}
              <Button size="sm" tone="subtle" icon="info" {...props}>{t('app.help.label')}</Button>
            {/snippet}
          </Menu>
          <Button size="sm" tone="subtle" icon="log-out" onclick={() => void session.signOut()}>
            {t('app.sign_out')}
          </Button>
        </Inline>
      </div>
    {/if}
  </header>

  <div class="notices">
    <Stack gap="150">
      {#if shouldAnnounce() && !dismissed}
        <!-- ADR-0035 §2: while the stage is not `stable` the application says so itself. The
             stage comes from `lib/maturity.ts` and from nowhere else. -->
        <Banner
          tone="info"
          title={t(`app.maturity.${MATURITY}.title`)}
          dismissLabel={t('app.dismiss')}
          onDismiss={() => (dismissed = true)}
        >
          {t(`app.maturity.${MATURITY}.body`)}
        </Banner>
      {/if}
      <!-- Nothing at all unless the reader may read the report and it says something is wrong. -->
      <HealthNotice />
      <!-- The copy and the server, on every route (F6-06): connected, reconnecting or offline;
           what waits to be sent and what the server refused; when the copy last synchronised.
           `SyncLine` feeds `SyncStatus` from the stream's state and the engine's queue. -->
      <div class="live">
        <SyncLine />
      </div>
    </Stack>
  </div>

  <div class="body">
    <!-- The sidebar is the frame's, not a view's: it is the same tree on every screen, and a view
         that rendered it would rebuild it on every navigation. -->
    {#if session.isSignedIn}
      <aside class="sidebar" data-tour="hubs">
        <WorkspaceNav currentId={route.params.id} {onnavigate} />
      </aside>
    {/if}
    <!-- `tabindex="-1"` so that the skip link has somewhere to land; a landmark is not a control,
         so it draws no ring when it does. -->
    <main id="main" tabindex="-1" bind:this={mainElement}>
      {@render children()}
    </main>
  </div>

  <!-- The one link out of the application (design-system.md §10, the statement): the accessibility
       statement lives on the website, unversioned, so that what it says about a walk is not tied
       to the build that shipped before the walk. A top-level navigation, which `connect-src` does
       not govern; a new tab, because the application is what the reader was in the middle of. -->
  <footer class="foot">
    <a href="https://hubtask.eu/accessibility/" target="_blank" rel="noopener">
      {t('app.footer.accessibility')}
    </a>
  </footer>

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
  .live { display: flex; }

  .frame {
    display: flex;
    flex-direction: column;
    gap: var(--sp-300);
    min-height: 100vh;
    padding: var(--sp-300);
    background: var(--bg-canvas);
    color: var(--text-primary);
  }

  .bar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--sp-300);
    padding-block-end: var(--sp-200);
    border-block-end: var(--bw-hairline) solid var(--border-subtle);
  }

  .wordmark {
    font-family: var(--font-display);
    font-size: var(--fs-300);
    font-weight: var(--fw-semibold);
    color: var(--text-primary);
    text-decoration: none;
  }

  nav ul {
    display: flex;
    flex-wrap: wrap;
    gap: var(--sp-200);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  nav a {
    color: var(--text-secondary);
    font-size: var(--fs-100);
    text-decoration: none;
  }

  nav a:hover { color: var(--text-primary); }

  /* The current page is not marked by colour alone (rule 3): `aria-current` carries it for a
     screen reader, and the underline carries it for everyone else. */
  nav a[aria-current='page'] {
    color: var(--text-primary);
    text-decoration: underline;
    text-underline-offset: var(--sp-050);
  }

  /* Rule 5, on every focusable thing in the frame. */
  a:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
    border-radius: var(--r-xs);
  }

  .notices:empty { display: none; }

  /* Pushed to the far edge of the bar, in both directions. */
  .actor { margin-inline-start: auto; }

  .who { color: var(--text-subtle); font-size: var(--fs-075); }

  .body { display: flex; flex: 1; gap: var(--sp-300); min-width: 0; }

  .foot {
    padding-block-start: var(--sp-200);
    border-block-start: var(--bw-hairline) solid var(--border-subtle);
    font-size: var(--fs-075);
    color: var(--text-subtle);
  }

  .foot a { color: inherit; }

  .foot a:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .sidebar {
    flex: none;
    /* Composed from the space scale rather than a width of its own: three of the largest step is
       a sidebar, and a number written here would be a value outside tokens.json (rule 15). */
    inline-size: calc(var(--sp-1000) * 3);
    max-inline-size: 40%;
    min-width: 0;
    border-inline-end: var(--bw-hairline) solid var(--border-subtle);
    padding-inline-end: var(--sp-200);
  }

  /* `expanded` is where the sidebar is pinned, and that is the token's own description rather than
     a width chosen here — `primitive.breakpoint.expanded` says "tablet landscape, sidebar pinned".
     Below it the tokens ask for the sidebar as an **overlay**, which is what `Drawer` is for; this
     stacks instead, which is honest and smaller, and the overlay belongs with the platform
     adaptation §9 still lists as open.

     A media query cannot read a custom property — `@media (max-width: var(--bp-expanded))` is not
     valid CSS in any engine — so the token's value is written out. It is
     `primitive.breakpoint.expanded` minus one and nothing else; the token remains the source. */
  /* design-system-lint-ignore: `primitive.breakpoint.expanded` (905px) less one; a media query cannot read a custom property. */
  @media (max-width: 904px) {
    .body { flex-direction: column; }

    .sidebar {
      inline-size: auto;
      max-inline-size: none;
      border-inline-end: 0;
      border-block-end: var(--bw-hairline) solid var(--border-subtle);
      padding-inline-end: 0;
      padding-block-end: var(--sp-200);
    }
  }

  main { flex: 1; min-width: 0; }

  main:focus { outline: none; }

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
