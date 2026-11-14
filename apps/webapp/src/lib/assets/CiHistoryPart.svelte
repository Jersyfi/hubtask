<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // An object's *History* (UC-AST-01 check 13; IN-2, scene `ci`): the entry's history for an
  // object, newest first, one sentence a step. The server sends codes and the fields that moved;
  // people, objects and files are named here from what the page already reads, stages and
  // verdicts in the reader's words, an attribute only by its name (its value is on the page).
  import { untrack } from 'svelte';
  import { ActivityFeed, FoldingSection, LoadMore, type ActivityStep } from '@hubtask/design-system/components';
  import type { ActivityEntry, CiHistoryPage, CiHistoryStep, ConfigurationItem, MediaObject } from '@hubtask/sync-engine';

  import { accounts } from '../data/accounts.svelte.ts';
  import { actor } from '../data/account.svelte.ts';
  import { actorCodes, sentenceCode } from '../data/activity.ts';
  import { groups } from '../data/groups.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { formatDateTime, formatPlainDay } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    ci: ConfigurationItem;
    /** The objects the page knows, by identifier: a relation's other end, a location. */
    known: ReadonlyMap<string, ConfigurationItem>;
    /** The object's files, for a step that names one. */
    files: readonly MediaObject[];
    /** The attribute's word, as the page says it. */
    word: (key: string) => string;
  }

  const { ci, known, files, word }: Props = $props();

  const historyPath = (id: string) => `/cis/${id}/history`;
  const page = resource<CiHistoryPage>(untrack(() => historyPath(ci.id)));
  const rows = $derived(page.state.status === 'ready' ? page.state.data.data : []);
  const hasMore = $derived(page.state.status === 'ready' && (page.state.data.page?.has_more ?? false));
  const failure = $derived(page.state.status === 'failed' ? renderProblem(page.state.error, messages).message : undefined);
  let arrived = $state<string | undefined>(undefined);

  // The people a step names, asked once.
  $effect(() => {
    const ids = rows.flatMap((step) => [
      step.actor.id,
      ...['owner_account_id'].flatMap((field) => {
        const moved = (step.change_set as Record<string, { from?: string; to?: string }>)[field];
        return [moved?.from, moved?.to];
      }),
    ]).filter((each): each is string => Boolean(each));
    if (ids.length > 0) untrack(() => accounts.resolve(ids));
  });

  /** A value as the reader knows it: a person, a group, an object, a file, a stage, a day. */
  function valueOf(field: string, value: string | undefined): string | undefined {
    if (!value) return undefined;
    switch (field) {
      case 'owner_account_id': return accounts.nameOf(value) ?? t('app.activity.actor_someone');
      case 'owner_group_id': return groups.all.find((each) => each.id === value)?.name ?? t('app.object.history_unknown');
      case 'location_id':
      case 'other_ci_id':
      case 'merged_into': return known.get(value)?.name ?? t('app.object.history_unknown');
      case 'media_id': return files.find((each) => each.id === value)?.file_name ?? t('app.object.history_file_gone');
      case 'stage': return messages.has(`app.assets.stage.${value}`) ? t(`app.assets.stage.${value}`) : value;
      case 'status': return messages.has(`app.assets.status.${value}`) ? t(`app.assets.status.${value}`) : value;
      case 'review_due': return formatPlainDay(value, messages.locale);
      case 'direction': return undefined;
      default: return value;
    }
  }

  function fieldOf(field: string): string {
    return messages.has(`app.object.history_field.${field}`) ? t(`app.object.history_field.${field}`) : word(field);
  }

  function stepOf(step: CiHistoryStep): ActivityStep {
    const who = actorCodes(step as unknown as ActivityEntry, actor.account?.id).find((code) => messages.has(code));
    const name = who === 'app.activity.actor_you' ? t('app.activity.actor_you') : accounts.nameOf(step.actor.id ?? undefined);
    const changes = Object.entries(step.change_set as Record<string, { from?: string; to?: string; changed?: boolean }>)
      .filter(([field]) => field !== 'direction')
      .map(([field, moved]) => {
        const from = valueOf(field, moved.from);
        const to = valueOf(field, moved.to);
        const detail = moved.changed ? t('app.activity.changed')
          : from && to ? t('app.activity.from_to', { from, to })
          : to ? t('app.activity.set_to', { to })
          : from ? t('app.activity.cleared_from', { from })
          : t('app.activity.no_detail');
        return { field: fieldOf(field), detail };
      });
    return {
      id: step.id,
      sentence: messages.has(step.code)
        ? t(sentenceCode(step.code, who === 'app.activity.actor_you', (code) => messages.has(code)), { actor: name ?? t(who ?? 'app.activity.actor_someone') })
        : step.code,
      when: formatDateTime(step.occurred_at, messages.locale),
      at: step.occurred_at,
      changes,
    };
  }

  const steps = $derived(rows.map(stepOf));

  async function more() {
    await page.loadMore();
    arrived = t('app.activity.arrived', { count: steps.length });
  }
</script>

<FoldingSection id="ci-history" title={t('app.object.mark_history')}>
  {#snippet children()}
    <div data-ci-history>
      {#if failure}<p class="failure" role="alert">{failure}</p>{/if}
      <ActivityFeed label={t('app.object.history_label', { name: ci.name })} {steps} emptyLabel={t('app.object.history_none')} />
      {#if hasMore || arrived}
        <LoadMore label={t('app.activity.more')} {hasMore} arrivedLabel={arrived} onLoadMore={() => void more()} />
      {/if}
    </div>
  {/snippet}
</FoldingSection>

<style>
  .failure { margin: 0; color: var(--text-danger); }
</style>
