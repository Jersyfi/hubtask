<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The first screen after signing in — and what it stopped being (ADR-0063 decision 1).
  //
  // It was a list of the hubs. The tree beside it lists the hubs, so on every width above a phone
  // this screen said what the navigation already said, and the walk of 2026-09-22 found nothing
  // here worth arriving at. It keeps its address and becomes **what is on the reader**: what of
  // theirs is overdue and what is due next, what waits in the jumble, what this device opened
  // last — and, for a workspace with no hub at all, the one action that starts one.
  //
  // **Composed of reads that already exist**, which is decision 4's rule and the reason this
  // screen took no server work. The first panel is one wordless search, which is a question only
  // since ADR-0064 (`lib/data/overview.svelte.ts`); the jumble is the same read the jumble screen
  // makes; what was opened last is this device's and is sent nowhere (`lib/recents.svelte.ts`).
  // The hubs are not read here at all — the frame already holds them for the tree.
  //
  // **Every panel says something when it is empty.** A panel that drew nothing would read as a
  // screen that had not finished loading, which is exactly what the reader cannot tell apart from
  // "there is nothing on you" — so each has a sentence, and the sentence is about that panel.

  import { untrack } from 'svelte';

  import { Button, EmptyState, ErrorState, ListRow, PageHeader, Skeleton, Stack, TaskRow } from '@hubtask/design-system/components';

  import DueMark from '../lib/entries/DueMark.svelte';

  import { actor } from '../lib/data/account.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { isOverdue } from '../lib/data/due.ts';
  import { jumble } from '../lib/data/jumble.svelte.ts';
  import { overview } from '../lib/data/overview.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { recents } from '../lib/recents.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import CreateContainerDialog from '../lib/workspace/CreateContainerDialog.svelte';

  interface Props {
    onnavigate?: (path: string) => void;
  }

  const { onnavigate }: Props = $props();

  let isCreatingHub = $state(false);

  $effect(() => page.entitle(t('app.nav.overview')));

  // The two reads this screen starts, and the only two. Both stop when it leaves.
  //
  // `untrack`, for the reason every store records: subscribing delivers the current state at once
  // and the listener writes the store, and writing it reads it - so an effect that both subscribes
  // and tracks that read re-runs on its own first delivery, for ever.
  $effect(() => untrack(() => overview.open()));
  $effect(() => untrack(() => jumble.open()));

  const zone = $derived(actor.zone);
  const now = $derived.by(() => Date.now());

  /**
   * The line between the two lists, drawn here rather than asked for twice.
   *
   * "Overdue" is the reader's question: it depends on their time zone and on whether a date is a
   * day or an instant, which `data/due.ts` is the one answer to. The server sorted them; this only
   * says where the first panel ends and the second begins.
   */
  const overdue = $derived(overview.mine.filter((item) => isOverdue(item, zone, now)));
  const next = $derived(overview.mine.filter((item) => !isOverdue(item, zone, now)));

  const waiting = $derived(jumble.of());

  const failure = $derived(
    overview.state.status === 'failed' ? renderProblem(overview.state.error, messages) : undefined,
  );

  const isLoading = $derived(overview.state.status === 'loading' || overview.state.status === 'idle');
</script>

<Stack gap="300">
  <PageHeader
    title={t('app.nav.overview')}
    isTitleInBar={viewport.isCompact}
    primary={{ label: t('app.workspace.create_hub'), icon: 'plus', onclick: () => (isCreatingHub = true), opener: 'add-hub' }}
  />

  {#if containers.hasNoHubs && containers.hubsState.status === 'ready'}
    <!-- A workspace with nothing in it has one thing to say and one thing to do. Not a panel of
         three empty sentences, which is what the rest of this screen would be. -->
    <EmptyState kind="unused" title={t('app.workspace.no_hubs')} icon="hub">
      {#snippet action()}
        <Button icon="plus" onclick={() => (isCreatingHub = true)}>{t('app.workspace.create_hub')}</Button>
      {/snippet}
    </EmptyState>
  {:else}
    <div class="panels">
      <!-- What is on the reader. One read, split in two where their own day divides it. -->
      <section class="panel" aria-labelledby="overview-mine">
        <Stack gap="150">
          <h2 class="head" id="overview-mine">{t('app.overview.mine')}</h2>
          {#if isLoading}
            <div aria-busy="true"><Skeleton lines={3} /></div>
          {:else if failure}
            <ErrorState
              title={failure.message}
              reference={failure.reference}
              referenceLabel={t('app.reference')}
              retryLabel={t('app.retry')}
              onRetry={() => overview.open()()}
            />
          {:else if overview.mine.length === 0}
            <p class="none">{t('app.overview.mine_none')}</p>
          {:else}
            {#if overdue.length > 0}
              <h3 class="band">{t('app.overview.overdue', { count: overdue.length })}</h3>
              <Stack gap="050">
                {#each overdue as item (item.id)}
                  <TaskRow
                    type={item.type}
                    title={item.title}
                    href={`/items/${item.id}`}
                    completeLabel={t('app.entries.complete', { title: item.title })}
                  >
                    {#snippet trailing()}<DueMark {item} />{/snippet}
                  </TaskRow>
                {/each}
              </Stack>
            {/if}
            {#if next.length > 0}
              <h3 class="band">{t('app.overview.next')}</h3>
              <Stack gap="050">
                {#each next as item (item.id)}
                  <TaskRow
                    type={item.type}
                    title={item.title}
                    href={`/items/${item.id}`}
                    completeLabel={t('app.entries.complete', { title: item.title })}
                  >
                    {#snippet trailing()}<DueMark {item} />{/snippet}
                  </TaskRow>
                {/each}
              </Stack>
            {/if}
            {#if overview.hasMore}
              <!-- `has_more`, not the length: this read answers what the caller may see rather
                   than refusing what they may not, so a short page is not the end of the list. -->
              <p class="more"><a href="/search">{t('app.overview.mine_more')}</a></p>
            {/if}
          {/if}
        </Stack>
      </section>

      <!-- What arrived and has not been put anywhere. The count is the point; three rows are the
           reminder that it is made of things. -->
      <section class="panel" aria-labelledby="overview-jumble">
        <Stack gap="150">
          <h2 class="head" id="overview-jumble">{t('app.overview.jumble')}</h2>
          {#if jumble.stateOf().status === 'loading' || jumble.stateOf().status === 'idle'}
            <div aria-busy="true"><Skeleton lines={2} /></div>
          {:else if waiting.length === 0}
            <p class="none">{t('app.overview.jumble_none')}</p>
          {:else}
            <Stack gap="050">
              {#each waiting.slice(0, 3) as entry (entry.id)}
                <!-- The same word the jumble screen puts on a row, and for the same reason: an
                     arriving message need not carry a subject, and "(no subject)" is what it is
                     called rather than a blank. -->
                <ListRow href="/jumble">{entry.raw_subject?.trim() || t('app.jumble.no_subject')}</ListRow>
              {/each}
            </Stack>
            <p class="more"><a href="/jumble">{t('app.overview.jumble_more', { count: waiting.length })}</a></p>
          {/if}
        </Stack>
      </section>

      <!-- What this device opened last. No read at all: it is kept here and sent nowhere. -->
      <section class="panel" aria-labelledby="overview-recent">
        <Stack gap="150">
          <h2 class="head" id="overview-recent">{t('app.overview.recent')}</h2>
          {#if recents.rows.length === 0}
            <p class="none">{t('app.overview.recent_none')}</p>
          {:else}
            <Stack gap="050">
              {#each recents.rows as row (row.kind + row.id)}
                <ListRow href={row.kind === 'item' ? `/items/${row.id}` : row.kind === 'hub' ? `/hubs/${row.id}` : `/collections/${row.id}`}>
                  {row.title}
                </ListRow>
              {/each}
            </Stack>
          {/if}
        </Stack>
      </section>
    </div>
  {/if}
</Stack>

<CreateContainerDialog bind:isOpen={isCreatingHub} type="HUB" oncreated={(id) => onnavigate?.(`/hubs/${id}`)} />

<style>
  /* One column on a phone, two from where there is room for two. The panels are equal citizens;
     none of them is a sidebar, so none of them is narrower than the others. */
  .panels {
    display: grid;
    gap: var(--sp-200);
    grid-template-columns: 1fr;
  }

  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width >= 600px) {
    .panels { grid-template-columns: repeat(2, minmax(0, 1fr)); }
    /* What is on the reader is the reason they are here, so it takes the width on a wide screen. */
    .panels > :global(.panel:first-child) { grid-column: 1 / -1; }
  }

  /* A standalone element, so it takes the surface and the raised shadow rather than a border
     (`design-system.md` rule 1). The frame's canvas is what it sits on. */
  .panel {
    background: var(--bg-surface);
    border-radius: var(--r-lg);
    box-shadow: var(--shadow-raised);
    padding: var(--sp-200);
  }

  .head {
    margin: 0;
    font-size: var(--fs-200);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-tight);
  }

  /* The band's word stands where the titles under it start (issue 1022). A heading at the row's
     own edge reads as a label on the panel rather than on the list under it, and "Overdue" is
     about those rows.

     The inset is the row's leading, in the tokens that draw it: `ListRow`'s inline padding, the
     twist, the completion box with its border, the type mark, and the three gaps between them.
     It is arithmetic over somebody else's component, so it is **measured** rather than trusted -
     `e2e/overview.test.mjs` asserts that a band and the first title under it share an x, and goes
     red if `TaskRow`'s leading ever changes. */
  .band {
    margin: 0;
    padding-inline-start: calc(
      var(--sp-150) + var(--sp-400) + var(--sp-250) + var(--bw-hairline) * 2 + var(--sp-400) +
        var(--density-row-gap) * 3
    );
    font-size: var(--fs-075);
    font-weight: var(--fw-medium);
    color: var(--text-secondary);
    text-transform: none;
  }

  /* An empty panel is a sentence, not a blank: the reader can tell "nothing here" from "not
     loaded yet", which is the acceptance this screen was written against. */
  .none { margin: 0; color: var(--text-secondary); font-size: var(--fs-100); }

  .more { margin: 0; font-size: var(--fs-075); }

  .more a { color: var(--text-brand); }

  .more a:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
    border-radius: var(--r-xs);
  }
</style>
