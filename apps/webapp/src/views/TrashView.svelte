<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What was deleted, what it takes with it, and how long there is to change your mind.
  //
  // **One row per deletion.** A hub with two hundred entries under it went in as one act and comes
  // back as one act (I-C2), so every row here is the *root* of a deletion and restoring it brings
  // the batch. There is deliberately nothing on this screen that restores one entry out of a batch:
  // the invariant the batch exists for is "restoring is atomic", and an offer that broke it would
  // be a control the model has no answer for.
  //
  // **Emptying is the one irreversible thing in this milestone**, and it is treated as one. It says
  // how many deletions and what goes with them, it is confirmed rather than offered as a plain
  // button, it carries the destructive tone, and it is never the default action of anything — the
  // dialog opens on Cancel's side and the confirm is named for what it does rather than "OK".
  //
  // **The window is stated where a person decides**, because "it will be gone anyway in a week"
  // changes the decision. Where the client may not read the workspace's retention policy it says
  // so rather than asserting the documented default as this installation's.

  import { untrack } from 'svelte';

  import {
    Badge,
    Button,
    Dialog,
    EmptyState,
    ErrorState,
    Inline,
    ListRow,
    Skeleton,
    Stack,
  } from '@hubtask/design-system/components';
  import type { TrashEntry } from '@hubtask/sync-engine';

  import { announcer } from '../lib/announce.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { items } from '../lib/data/items.svelte.ts';
  import { isContainer, remainingDays } from '../lib/data/lifecycle.ts';
  import { retention } from '../lib/data/retention.svelte.ts';
  import { trash } from '../lib/data/trash.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  // `untrack` for the reason every store here records: the load writes the store and writing it
  // reads it, so an effect that tracks that read re-runs on its own first answer.
  $effect(() => {
    void untrack(() => {
      void trash.load();
      void retention.start();
    });
  });

  const now = Date.now();

  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let busyId = $state<string | undefined>(undefined);
  /** Which row the reader is being asked about, and whether they are being asked about all of it. */
  let purging = $state<TrashEntry | undefined>(undefined);
  let isEmptying = $state(false);
  let isWorking = $state(false);
  /** What the last pass of an empty did, kept until the reader has read it. */
  let summary = $state<{ matched: number; removed: number; blocked: Record<string, number> } | undefined>(
    undefined,
  );

  const listFailure = $derived(
    trash.status === 'failed' && trash.error ? renderProblem(trash.error, messages) : undefined,
  );

  /**
   * The window, or nothing. `undefined` days is not "0 days" and must not read like it.
   *
   * The sentence names the number rather than inflecting around it, because the source catalogue
   * stays inside the **simple-argument** subset: `infrastructure/i18n/Catalogue_test.go` holds the
   * whole file to it, so that the day a message needs a plural somebody teaches the Go renderer
   * rather than watching it print braces at a reader.
   */
  function window(row: TrashEntry): string {
    const left = remainingDays(row.deleted_at, retention.trashDays, now);
    if (left === undefined) return t('app.trash.window_unknown');
    return left === 0 ? t('app.trash.window_last') : t('app.trash.window', { days: left });
  }

  async function restore(row: TrashEntry) {
    failure = undefined;
    busyId = row.id;
    try {
      // Which endpoint is the row's own answer: the trash mixes containers and entries by design.
      if (isContainer(row.kind)) await containers.restore(row.id, crypto.randomUUID());
      else await items.restore(row.id, crypto.randomUUID());
      announcer.say(t('app.trash.restored', { title: row.title }));
      await trash.load();
    } catch (error) {
      failure = renderProblem(error as never, messages);
    } finally {
      busyId = undefined;
    }
  }

  /**
   * Destroys one deletion for good.
   *
   * Only an **entry** can be purged on its own — the contract has `:purge` for an item and none
   * for a container, so a container in the trash goes when the trash is emptied or when the
   * retention job reaches it. The control says why rather than being absent.
   */
  async function purge(row: TrashEntry) {
    failure = undefined;
    isWorking = true;
    try {
      await items.purge(row.id, crypto.randomUUID());
      announcer.say(t('app.trash.purged', { title: row.title }));
      purging = undefined;
      await trash.load();
    } catch (error) {
      // A legal hold refuses this, and `lifecycle.legal_hold` is its own sentence naming the scope
      // — which is `problem.ts` choosing the detail code over the category, not a branch here.
      failure = renderProblem(error as never, messages);
    } finally {
      isWorking = false;
    }
  }

  async function empty() {
    failure = undefined;
    isWorking = true;
    try {
      const pass = await trash.empty(crypto.randomUUID());
      summary = {
        matched: pass.matched,
        removed: pass.removed,
        blocked: (pass.blocked ?? {}) as Record<string, number>,
      };
      isEmptying = false;
    } catch (error) {
      failure = renderProblem(error as never, messages);
    } finally {
      isWorking = false;
    }
  }
</script>

<Stack gap="300">
  <h1 class="name">{t('app.trash.title')}</h1>

  {#if failure}<p class="failure">{failure.message}</p>{/if}

  {#if summary}
    <div class="summary">
      <p>{t('app.trash.summary', { removed: summary.removed, matched: summary.matched })}</p>
      <!-- What was **kept**, and why. A legal hold does not fail the call; it is counted in the
           answer, and an answer that reported only a number would leave the reader wondering
           which of their deletions is still there. -->
      {#each Object.entries(summary.blocked) as [reason, count] (reason)}
        {#if count > 0}
          <p>{t('app.trash.kept', { count, reason: t(`app.trash.blocked.${reason}`) })}</p>
        {/if}
      {/each}
      {#if summary.matched > summary.removed && Object.keys(summary.blocked).length === 0}
        <p>{t('app.trash.again')}</p>
      {/if}
      <div>
        <Button size="sm" tone="secondary" onclick={() => (summary = undefined)}>
          {t('app.dismiss')}
        </Button>
      </div>
    </div>
  {/if}

  {#if listFailure}
    <ErrorState
      title={listFailure.message}
      reference={listFailure.reference}
      referenceLabel={t('app.reference')}
      retryLabel={t('app.retry')}
      onRetry={() => trash.load()}
    />
  {:else if trash.status === 'loading' && trash.rows.length === 0}
    <div aria-busy="true"><Skeleton lines={4} /></div>
  {:else if trash.rows.length === 0}
    <!-- `settled` rather than `unused`: an empty trash is the good outcome, not a thing nobody
         has got round to — and §4.3 offers nothing to do about it, which is what that kind
         refuses a call to action for. -->
    <EmptyState kind="settled" title={t('app.trash.empty')} icon="trash-2" />
  {:else}
    <Stack gap="050">
      {#each trash.rows as row (row.id)}
        <ListRow>
          {#snippet leading()}
            <Badge>{row.subtype}</Badge>
          {/snippet}
          <span class="row">
            <span class="title">{row.title}</span>
            <span class="detail">
              {t('app.trash.deleted_at', { when: formatDateTime(row.deleted_at, messages.locale) })}
              · {window(row)}
              · {t('app.trash.batch')}
            </span>
          </span>
          {#snippet trailing()}
            <Inline gap="100">
              <Button
                size="sm"
                tone="secondary"
                isBusy={busyId === row.id}
                busyLabel={t('app.trash.restore')}
                onclick={() => restore(row)}
              >
                {t('app.trash.restore')}
              </Button>
              <Button
                size="sm"
                tone="danger"
                onclick={() => (purging = row)}
                disabledReason={isContainer(row.kind) ? t('app.trash.purge_container') : undefined}
              >
                {t('app.trash.purge')}
              </Button>
            </Inline>
          {/snippet}
        </ListRow>
      {/each}
    </Stack>

    {#if trash.isPartial}<p class="detail">{t('app.trash.partial')}</p>{/if}

    <div>
      <Button tone="danger" onclick={() => (isEmptying = true)}>{t('app.trash.empty_all')}</Button>
    </div>
  {/if}
</Stack>

{#if purging}
  <Dialog
    isOpen={true}
    title={t('app.trash.purge_title', { title: purging.title })}
    dismissLabel={t('app.workspace.cancel')}
    onClose={() => (purging = undefined)}
  >
    <p class="detail">{t('app.trash.purge_body')}</p>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (purging = undefined)}>
        {t('app.workspace.cancel')}
      </Button>
      <Button
        tone="danger"
        isBusy={isWorking}
        busyLabel={t('app.trash.purge_confirm')}
        onclick={() => purging && purge(purging)}
      >
        {t('app.trash.purge_confirm')}
      </Button>
    {/snippet}
  </Dialog>
{/if}

{#if isEmptying}
  <Dialog
    bind:isOpen={isEmptying}
    title={t('app.trash.empty_title')}
    dismissLabel={t('app.workspace.cancel')}
  >
    <!-- It says how many and what goes with them, because nothing here deletes without the person
         having read what it deletes. -->
    <p class="detail">{t('app.trash.empty_body', { count: trash.rows.length })}</p>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isEmptying = false)}>
        {t('app.workspace.cancel')}
      </Button>
      <Button
        tone="danger"
        isBusy={isWorking}
        busyLabel={t('app.trash.empty_confirm')}
        onclick={empty}
      >
        {t('app.trash.empty_confirm')}
      </Button>
    {/snippet}
  </Dialog>
{/if}

<style>
  .name {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--fs-400);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-tight);
  }

  .row { display: flex; flex-direction: column; gap: var(--sp-025); }

  .title { overflow-wrap: anywhere; }

  .detail { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); max-width: 64ch; }

  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); max-width: 64ch; }

  .summary {
    display: flex;
    flex-direction: column;
    gap: var(--sp-100);
    max-width: 64ch;
    padding: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    background: var(--bg-surface);
    box-shadow: var(--shadow-raised);
    font-size: var(--fs-075);
  }

  .summary p { margin: 0; }
</style>
