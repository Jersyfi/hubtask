<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One entry, at the address the board and the search results already linked to.
  //
  // **The history is the point of this screen** (F2-15), and the rule that shapes it is
  // `domain-model.md` §3.5: the server stores `item.completed` and sends
  // `activity.item_completed`, and the client renders it. Nothing here writes a verb; the
  // catalogue does, and a code this client has never heard of still reads as words because
  // `messages.t` humanises an unknown code rather than printing a key — which is the normal state
  // of a client one milestone behind its server rather than an error.
  //
  // The **actor** is the one place this screen refuses to guess. The contract says of an activity
  // actor that "the label is not here: the account is one request away" — and for anybody but the
  // signed-in account, that request does not exist: `/accounts/me` is the only account read
  // declared. So the reader is "You" and everybody else is named by their kind.

  import { untrack } from 'svelte';

  import {
    ActivityFeed,
    Badge,
    EmptyState,
    ErrorState,
    LoadMore,
    Skeleton,
    Stack,
    type ActivityStep,
  } from '@hubtask/design-system/components';
  import type { ActivityEntry, ActivityPage, WorkItem } from '@hubtask/sync-engine';

  import { actor } from '../lib/data/account.svelte.ts';
  import { actorCodes, changesOf } from '../lib/data/activity.ts';
  import { activityPath, itemPath } from '../lib/data/item.svelte.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  interface Props {
    id: string;
  }

  const { id }: Props = $props();

  // Read once, and `untrack` says the once is deliberate: `App.svelte` keys this view on the id,
  // so a different entry is a different component rather than the same one asking again. A
  // resource that followed the prop would be a second answer to which entry this screen is.
  const entry = resource<WorkItem>(untrack(() => itemPath(id)));
  const history = resource<ActivityPage>(untrack(() => activityPath(id)));

  const item = $derived(entry.state.status === 'ready' ? entry.state.data : undefined);
  const failure = $derived(
    entry.state.status === 'failed' ? renderProblem(entry.state.error, messages) : undefined,
  );

  /** What the history kept about one field, as one phrase. */
  function detailOf(change: ReturnType<typeof changesOf>[number]): string {
    // A field whose values the history does not keep says so and nothing else. A note is the
    // worked example, and looking for its text would be looking for what ADR-0017 kept out.
    if (change.isOpaque) return t('app.activity.changed');
    if (change.from !== undefined && change.to !== undefined) {
      return t('app.activity.from_to', { from: change.from, to: change.to });
    }
    if (change.to !== undefined) return t('app.activity.set_to', { to: change.to });
    if (change.from !== undefined) return t('app.activity.cleared_from', { from: change.from });
    return t('app.activity.no_detail');
  }

  function stepOf(step: ActivityEntry): ActivityStep {
    // The first code the catalogue knows, which for an actor kind this client has never heard of
    // is the sentence true of every actor. The same shape `problem.ts` uses for a problem's codes.
    const who = actorCodes(step, actor.account?.id).find((code) => messages.has(code));
    return {
      id: step.id,
      // The verb is the server's code and the actor is a parameter of it. An unrecognised verb
      // renders as `humanise` makes of it — readable, never a key and never a blank.
      sentence: t(step.code, { actor: t(who ?? 'app.activity.actor_someone') }),
      when: formatDateTime(step.occurred_at, messages.locale),
      at: step.occurred_at,
      changes: changesOf(step.change_set as Record<string, unknown>).map((change) => ({
        field: change.field,
        detail: detailOf(change),
      })),
    };
  }

  const steps = $derived(
    history.state.status === 'ready'
      ? (history.state.data.data ?? []).map(stepOf)
      : ([] as ActivityStep[]),
  );
  const hasMore = $derived(
    history.state.status === 'ready' && (history.state.data.page?.has_more ?? false),
  );
  const historyFailure = $derived(
    history.state.status === 'failed' ? renderProblem(history.state.error, messages) : undefined,
  );
</script>

{#if entry.state.status === 'loading' || entry.state.status === 'idle'}
  <div aria-busy="true"><Skeleton lines={3} /></div>
{:else if failure}
  <ErrorState
    title={failure.message}
    reference={failure.reference}
    referenceLabel={t('app.reference')}
    retryLabel={t('app.retry')}
    onRetry={() => entry.refresh()}
  />
{:else if !item}
  <EmptyState kind="filtered" title={t('app.item.not_found')} />
{:else}
  <Stack gap="300">
    <Stack gap="150">
      <h1 class="name">{item.title}</h1>
      <div class="marks">
        <Badge>{item.type}</Badge>
        {#if item.archived_at}
          <Badge icon="archive">{t('app.entries.archived_label')}</Badge>
        {/if}
        {#if item.completion?.is_completed}
          <Badge tone="success">{t('app.entries.complete', { title: item.title })}</Badge>
        {/if}
      </div>
      {#if item.notes}<p class="notes">{item.notes}</p>{/if}
    </Stack>

    <Stack gap="150">
      <h2 class="section">{t('app.activity.title')}</h2>

      {#if historyFailure}
        <ErrorState
          title={historyFailure.message}
          reference={historyFailure.reference}
          referenceLabel={t('app.reference')}
          retryLabel={t('app.retry')}
          onRetry={() => history.refresh()}
        />
      {:else if history.state.status === 'loading' || history.state.status === 'idle'}
        <div aria-busy="true"><Skeleton lines={3} /></div>
      {:else}
        <ActivityFeed
          label={t('app.activity.label')}
          {steps}
          emptyLabel={t('app.activity.none')}
        />
        <!-- Cursor pagination, never a page number: the API has none, so no component may imply
             one. What arrived is announced, because pressing a button and being told nothing is
             the case a live region is for. -->
        {#if hasMore}
          <LoadMore
            label={t('app.activity.more')}
            arrivedLabel={t('app.activity.arrived', { count: steps.length })}
            onLoadMore={() => history.loadMore()}
          />
        {/if}
      {/if}
    </Stack>
  </Stack>
{/if}

<style>
  .name {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--fs-400);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-tight);
    overflow-wrap: anywhere;
  }

  .section {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--fs-300);
    font-weight: var(--fw-semibold);
  }

  .marks { display: flex; flex-wrap: wrap; gap: var(--sp-100); }

  .notes { margin: 0; max-width: 64ch; color: var(--text-secondary); white-space: pre-wrap; }
</style>
