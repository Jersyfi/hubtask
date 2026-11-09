<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One configuration version as its record page (page-system §9.3): when, by whom and from which
  // template; what it changed against the version before it - or another, chosen with *Compare
  // with…* - each block opening its Dialog L; *Roll back…* in the head's ⋯, a Dialog M with its
  // plan. Without STRUCTURE nothing is offered (P-05).
  import { untrack } from 'svelte';
  import { DataTable, EmptyState, RecordPage, Spinner, type DataColumn, type RecordList } from '@hubtask/design-system/components';
  import type { ConfigTemplateList, ConfigVersion, ConfigVersionList, ResourceState } from '@hubtask/sync-engine';

  import CompareDialog from '../lib/changes/CompareDialog.svelte';
  import DifferenceDialog from '../lib/changes/DifferenceDialog.svelte';
  import RollbackDialog from '../lib/changes/RollbackDialog.svelte';
  import { countsOf, templateNameOf } from '../lib/changes/words.ts';
  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import { config } from '../lib/data/config.svelte.ts';
  import { compareDocuments, differenceWords, type BlockDifference } from '../lib/data/configdiff.ts';
  import { versionByCode } from '../lib/data/configversion.ts';
  import FailureNote from '../lib/frame/FailureNote.svelte';
  import { page } from '../lib/frame/page.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  interface Props {
    version: string;
    onnavigate: (path: string) => void;
  }

  const { version, onnavigate }: Props = $props();

  const LIST = '/administration/changes';
  const number = $derived(Number(version));

  let versions = $state<ResourceState<ConfigVersionList>>();
  let catalogue = $state<ResourceState<ConfigTemplateList>>();
  $effect(() => untrack(() => config.versions((next) => (versions = next))));
  $effect(() => untrack(() => config.templates((next) => (catalogue = next))));

  const all = $derived(versions?.status === 'ready' ? versions.data.data : []);
  const record = $derived(all.find((each) => each.version === number));
  const isNewest = $derived(all[0]?.version === number);
  const mayChange = $derived(holdsHere('STRUCTURE') === true);
  const title = $derived(t('app.changes.version', { version: number }));
  $effect(() => page.entitle(title));

  // What it changed: against the version before it, unless another was chosen.
  let against = $state('');
  const previous = $derived(all.find((each) => each.version < number)?.version);
  const otherVersion = $derived(against !== '' ? Number(against) : previous);
  let compared = $state<BlockDifference[]>();
  let compareError = $state<string>();
  let isComparing = $state(false);
  $effect(() => {
    const other = otherVersion;
    if (Number.isNaN(number)) return;
    untrack(() => void compare(other));
  });

  async function compare(other: number | undefined): Promise<void> {
    isComparing = true;
    compareError = undefined;
    try {
      const documentOf = (state: ResourceState<ConfigVersion>) => (state.status === 'ready' ? state.data.document : undefined);
      const mine = await config.version(number);
      const theirs = other === undefined ? undefined : await config.version(other);
      const failed = mine.status === 'failed' ? mine : theirs?.status === 'failed' ? theirs : undefined;
      if (failed) {
        compareError = renderProblem(failed.error, messages).message;
        compared = undefined;
        return;
      }
      const [older, newer] = other !== undefined && other > number ? [documentOf(mine), theirs && documentOf(theirs)] : [theirs && documentOf(theirs), documentOf(mine)];
      compared = compareDocuments(older as never, newer as never);
    } finally {
      isComparing = false;
    }
  }

  const sides = $derived(otherVersion !== undefined && otherVersion > number ? [String(number), String(otherVersion)] : [String(otherVersion ?? ''), String(number)]);
  const wordsOf = (difference: BlockDifference) => {
    const said = differenceWords(difference);
    return t(said.code, { section: t(said.section), key: said.module ? t(said.module) : said.key });
  };

  const columns = $derived<DataColumn[]>([
    { id: 'what', label: t('app.changes.what_column'), priority: 1 },
    { id: 'kind', label: t('app.changes.state_column'), priority: 2, isStatus: true },
  ]);

  const lists = $derived<RecordList[]>([
    {
      id: 'changes',
      title: otherVersion !== undefined ? t('app.changes.changed_against', { version: otherVersion }) : t('app.changes.changed_first'),
      count: compared ? String(compared.length) : undefined,
      add: all.length > 1 ? { label: t('app.changes.compare_open'), opener: 'compare-with', onclick: () => (isChoosing = true) } : undefined,
    },
  ]);

  let opened = $state<BlockDifference | undefined>(undefined);
  let rollingBack = $state<number | undefined>(undefined);
  let isChoosing = $state(false);
  const compareOptions = $derived(all.filter((each) => each.version !== number).map((each) => ({ value: String(each.version), label: t('app.changes.version', { version: each.version }) })));

  const facts = $derived(
    record
      ? [
          formatDateTime(record.created_at, messages.locale),
          t(versionByCode(record)),
          countsOf(record),
          record.template ? t('app.changes.from_template', { name: templateNameOf(catalogue, record.template) }) : undefined,
          record.proposed_by ? t('app.changes.was_proposed') : undefined,
          record.rolled_back_to ? t('app.changes.rollback_of', { version: record.rolled_back_to }) : undefined,
        ].filter(Boolean).join(' · ')
      : undefined,
  );
  const mayRollBack = $derived(mayChange && record !== undefined && !isNewest);
</script>

<RecordPage
  record={{
    kind: t('app.changes.version_kind'),
    title,
    back: { label: t('app.changes.title'), href: LIST, onnavigate: () => onnavigate(LIST) },
    facts,
    menu: mayRollBack ? { label: t('app.page.more_for', { name: title }), items: [{ id: 'rollback', label: t('app.changes.roll_back_menu') }], onselect: () => (rollingBack = number) } : undefined,
  }}
  {lists}
  refused={versions?.status === 'failed' || (versions?.status === 'ready' && !record) ? notHere : undefined}
>
  {#snippet list()}
    {#if compareError}
      <FailureNote problem={compareError} />
    {:else if !compared || isComparing}
      <p class="quiet"><Spinner label={t('app.changes.reading')} /> <span>{t('app.changes.reading')}</span></p>
    {:else}
      <DataTable
        label={t('app.changes.compare')}
        {columns}
        rows={compared}
        key={(each: BlockDifference) => each.path}
        onopen={(each: BlockDifference) => (opened = each)}
        rowLabel={wordsOf}
      >
        {#snippet cell(each: BlockDifference, column: DataColumn)}
          {#if column.id === 'what'}
            <span data-difference-row={each.path}>{wordsOf(each)}</span>
          {:else}
            <span class="quiet">{t(`app.changes.difference_${each.kind.toLowerCase()}`)}</span>
          {/if}
        {/snippet}
        {#snippet empty()}<EmptyState kind="settled" title={t('app.changes.compare_same')} />{/snippet}
      </DataTable>
    {/if}
  {/snippet}
</RecordPage>

<DifferenceDialog difference={opened} from={sides[0] ?? ''} to={sides[1] ?? ''} onClose={() => (opened = undefined)} />
<RollbackDialog version={rollingBack} onClose={() => (rollingBack = undefined)} />
<CompareDialog bind:isOpen={isChoosing} options={compareOptions} value={String(otherVersion ?? '')} onchoose={(value) => (against = value)} />

{#snippet notHere()}
  {#if versions?.status === 'failed'}
    <FailureNote problem={renderProblem(versions.error, messages)} />
  {:else}
    <EmptyState kind="settled" title={t('app.changes.version_gone')} />
  {/if}
{/snippet}

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
</style>
