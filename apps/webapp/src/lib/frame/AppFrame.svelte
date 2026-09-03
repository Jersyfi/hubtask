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

  import { Banner, Button, Inline, Stack } from '@hubtask/design-system/components';

  import HealthNotice from './HealthNotice.svelte';

  import { actor } from '../data/account.svelte.ts';
  import { session } from '../session.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { MATURITY, shouldAnnounce } from '../maturity.ts';
  import type { Resolution } from '../router.ts';

  interface Props {
    route: Resolution;
    children: Snippet;
  }

  const { route, children }: Props = $props();

  // Dismissed for as long as this page is open, and no longer. ADR-0035 §2 asks for a banner that
  // is not in the way; it does not ask the client to remember a decision across visits, and a
  // client that did would need somewhere to keep it - which is the platform seam's question and
  // F6's storage port, not this component's.
  let dismissed = $state(false);

  // Who is signed in, read once the frame is up - and read again whenever that changes, which is
  // what the `session.status` below is doing in an effect that otherwise depends on nothing. A
  // subscription taken before a sign-in would be a subscription the sign-out already dropped.
  $effect(() => {
    void session.status;
    return actor.start();
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

  const links = [
    { path: '/', name: 'home', label: 'app.nav.home' },
    { path: '/installation', name: 'installation', label: 'app.nav.installation' },
  ];
</script>

<div class="frame">
  <header class="bar">
    <!-- A name rather than a message: the product is called Hubtask in every language. -->
    <a class="wordmark" href="/">Hubtask</a>
    <!-- Signed out there is nowhere to go but the token screen, so the frame offers nothing that
         would land there under another name. -->
    {#if session.isSignedIn}
      <nav aria-label={t('app.nav.home')}>
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
          <Button size="sm" tone="subtle" icon="log-out" onclick={() => session.signOut()}>
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
    </Stack>
  </div>

  <main>
    {@render children()}
  </main>
</div>

<style>
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

  main { flex: 1; }
</style>
