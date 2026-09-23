<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What the application has to say **about itself**, as one mark in the bar (ADR-0065 decision 4).
  //
  // Three things qualify, and the rule that lets them in is that they would say the same thing on
  // every screen: the maturity stage, which ADR-0035 §2 requires the application to state while it
  // is not `stable`; the health report, where the reader may read one and it says something is
  // wrong; and **the manifest this client could not read**, with the way to ask again. The first
  // two were banners above the page's own head - a statement about a *release* drawn where a
  // statement about the *page* belongs, taking a row of every screen with it.
  //
  // The third came from `SyncStatus` (issue 1020), which 1022's own "what this does not fix" left
  // open because this mark did not exist yet. It is a statement about the application rather than
  // about this copy's changes - but what decides it is simpler than the category: **`SyncLine` is
  // drawn only with a session, this mark always**, and `/meta/capabilities` is read before
  // anybody signs in. A retry behind a mark that is not on the screen is no retry.
  //
  // It is the pattern the connection's mark already is (ADR-0063 decision 5): unpressed it says
  // only that there is something; pressed it says all of it. The stage is not dismissible any
  // more, and that is a simplification rather than a loss - the dismiss existed because the banner
  // was in the way, and nothing that is not in the way needs pushing out of it.
  //
  // A notice about **the page** - a refused write, a check's findings - stays where `PageHeader`
  // draws it. This is for what is true wherever the reader stands.

  import { Button, Drawer, IconButton, Popover, Stack } from '@hubtask/design-system/components';

  import { viewport } from './viewport.svelte.ts';

  import { manifest } from '../data/capabilities.svelte.ts';
  import { health } from '../data/health.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { MATURITY, shouldAnnounce } from '../maturity.ts';
  import { renderProblem } from '../problem.ts';
  import { session } from '../session.svelte.ts';

  /**
   * The health report, subscribed for as long as this mark is drawn and read again when the
   * session changes: without a bearer there is nothing to read, and the subscription taken before
   * somebody signed in is one the sign-out already dropped.
   *
   * **The manifest deliberately has no effect here.** Both are read again when the actor changes,
   * by two mechanisms, because they are two kinds of read: a *subscription* is started and stopped
   * by whoever draws it, which is this component; a *one-shot* read is refreshed by whoever changed
   * the actor, which is `session.svelte.ts` - it calls `manifest.refresh()` at each of its four
   * transitions (issue 1020). A second refresh here would be this component asking again for a
   * read it does not own.
   */
  $effect(() => {
    void session.status;
    return health.start();
  });

  /** Whether the stage is worth stating. `lib/maturity.ts` is the one place that decides. */
  const hasStage = $derived(shouldAnnounce());
  /** Whether the report says something is wrong. Nothing at all without a report or a right to it. */
  const isUnwell = $derived(health.isTroubled);
  /**
   * The manifest, where it could not be read. Absent in the ordinary case, which is every case but
   * one - and the one is the whole application running on nothing it was told.
   *
   * **`failure`, not `state`**: the engine re-reads an unanswered resource on every reconnect and
   * publishes `loading` over the error, so a mark that read the live state would flicker between
   * the sentence and nothing - and the retry would go out from under the finger pressing it.
   */
  const unread = $derived.by(() => {
    if (!manifest.failure) return undefined;
    const problem = renderProblem(manifest.failure, messages);
    return { reason: problem.message, reference: problem.reference };
  });
  /** Anything wrong: the report, or an installation this client never read. */
  const isTroubled = $derived(isUnwell || unread !== undefined);
  const hasAnything = $derived(hasStage || isTroubled);

  /**
   * The mark, and what it says without being pressed.
   *
   * The ordinary case is the quiet mark: there is something to read and nothing is wrong. A
   * degraded or down installation is the one state somebody has to see before they ask, so it
   * takes the dot and the tone - rule 3, which asks for a mark and a word rather than a colour.
   */
  const label = $derived(
    unread ? t('app.installation.unread') : isUnwell ? t('app.health.title') : t(`app.maturity.${MATURITY}.title`),
  );

  let isOpen = $state(false);
</script>

{#snippet inside()}
  <div class="panel">
    <Stack gap="200">
      <!-- What this client could not read about the installation, first: a client that has not read
           the manifest knows no type, no role and no limit, so everything else it says is said on
           nothing. The ask-again is here because this mark is on every screen, signed in or not. -->
      {#if unread}
        <section class="notice" data-tone="danger" aria-label={t('app.installation.unread')}>
          <Stack gap="050">
            <h3 class="heading">{t('app.installation.unread')}</h3>
            {#if unread.reason}<p class="line">{unread.reason}</p>{/if}
            {#if unread.reference}
              <p class="line reference">{t('app.reference')} <code>{unread.reference}</code></p>
            {/if}
            <div>
              <Button size="sm" tone="secondary" icon="repeat" onclick={() => void manifest.refresh()}>
                {t('app.retry')}
              </Button>
            </div>
          </Stack>
        </section>
      {/if}
      <!-- Then the report: what is wrong outranks what the release promises. -->
      {#if isUnwell}
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

  /* The correlation id, in the data style: it is quoted into a support thread, not read. */
  .reference { color: var(--text-subtle); font-family: var(--font-mono); }
</style>
