<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What the application has to say **about itself**, as one mark in the bar (ADR-0065 decision 4).
  //
  // Two things qualify, and the rule that lets them in is that they would say the same thing on
  // every screen: the maturity stage, which ADR-0035 §2 requires the application to state while it
  // is not `stable`, and the health report, where the reader may read one and it says something is
  // wrong. Both were banners above the page's own head - a statement about a *release* drawn where
  // a statement about the *page* belongs, taking a row of every screen with it.
  //
  // It is the pattern the connection's mark already is (ADR-0063 decision 5): unpressed it says
  // only that there is something; pressed it says all of it. The stage is not dismissible any
  // more, and that is a simplification rather than a loss - the dismiss existed because the banner
  // was in the way, and nothing that is not in the way needs pushing out of it.
  //
  // A notice about **the page** - a refused write, a check's findings - stays where `PageHeader`
  // draws it. This is for what is true wherever the reader stands.

  import { Drawer, IconButton, Popover, Stack } from '@hubtask/design-system/components';

  import { viewport } from './viewport.svelte.ts';

  import { health } from '../data/health.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import { MATURITY, shouldAnnounce } from '../maturity.ts';
  import { session } from '../session.svelte.ts';

  // Read again when the session changes: without a bearer there is nothing to read, and the
  // subscription taken before somebody signed in is one the sign-out already dropped.
  $effect(() => {
    void session.status;
    return health.start();
  });

  /** Whether the stage is worth stating. `lib/maturity.ts` is the one place that decides. */
  const hasStage = $derived(shouldAnnounce());
  /** Whether the report says something is wrong. Nothing at all without a report or a right to it. */
  const isTroubled = $derived(health.isTroubled);
  const hasAnything = $derived(hasStage || isTroubled);

  /**
   * The mark, and what it says without being pressed.
   *
   * The ordinary case is the quiet mark: there is something to read and nothing is wrong. A
   * degraded or down installation is the one state somebody has to see before they ask, so it
   * takes the dot and the tone - rule 3, which asks for a mark and a word rather than a colour.
   */
  const label = $derived(isTroubled ? t('app.health.title') : t(`app.maturity.${MATURITY}.title`));

  let isOpen = $state(false);
</script>

{#snippet inside()}
  <div class="panel">
    <Stack gap="200">
      <!-- The report first where there is one: what is wrong outranks what the release promises. -->
      {#if isTroubled}
        <section class="notice" data-tone={health.isDown ? 'danger' : 'warning'} aria-label={t('app.health.title')}>
          <Stack gap="050">
            <h3 class="heading">{t('app.health.title')}</h3>
            {#each health.degradations as degradation (degradation.feature)}
              <p class="line">{t('app.health.feature', { feature: degradation.feature, reason: t(degradation.reasonCode) })}</p>
            {/each}
          </Stack>
        </section>
      {/if}
      {#if hasStage}
        <section class="notice" aria-label={t(`app.maturity.${MATURITY}.title`)}>
          <Stack gap="050">
            <h3 class="heading">{t(`app.maturity.${MATURITY}.title`)}</h3>
            <p class="line">{t(`app.maturity.${MATURITY}.body`)}</p>
          </Stack>
        </section>
      {/if}
    </Stack>
  </div>
{/snippet}

{#snippet mark(props: Record<string, unknown>)}
  <span class="mark" data-troubled={isTroubled ? '' : undefined}>
    <IconButton icon={isTroubled ? 'triangle-alert' : 'info'} {label} size="sm" {...props} />
    {#if isTroubled}<span class="dot" aria-hidden="true"></span>{/if}
  </span>
{/snippet}

{#if hasAnything}
  <div class="notice-mark">
    {#if viewport.isCompact}
      <!-- The same choice `SyncStatus` and the account group make: on a phone there is no room
           beside a bar's control, so what it opens comes from the edge. -->
      {@render mark({ onclick: () => (isOpen = true), 'aria-expanded': isOpen, 'aria-haspopup': 'dialog' })}
      <Drawer bind:isOpen edge="block-end" title={label} dismissLabel={t('app.dismiss')}>
        {@render inside()}
      </Drawer>
    {:else}
      <Popover label={label}>
        {#snippet trigger(props)}{@render mark(props)}{/snippet}
        {@render inside()}
      </Popover>
    {/if}
  </div>
{/if}

<style>
  .notice-mark { display: inline-flex; align-items: center; }

  /* A floor on the width, so the surface beside a bar's control is not squeezed into a column of
     two words by the edge it is anchored against. */
  .panel { min-inline-size: 28ch; max-inline-size: 44ch; }

  .mark { position: relative; display: inline-flex; }

  /* Rule 3 again: the state is the mark, the dot *and* the accessible name, never the colour
     alone - the mark turns into the warning triangle and the name becomes the report's title. */
  .mark[data-troubled] { color: var(--text-warning); }

  .dot {
    position: absolute;
    inset-block-start: 0;
    inset-inline-end: 0;
    inline-size: var(--sp-100);
    block-size: var(--sp-100);
    border-radius: var(--r-full);
    background: var(--status-warning-accent);
  }

  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); color: var(--text-primary); }

  .line { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }

  .notice[data-tone='danger'] .heading { color: var(--text-danger); }

  .notice[data-tone='warning'] .heading { color: var(--text-warning); }
</style>
