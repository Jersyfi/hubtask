<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One entry type as its record page (page-system §5.1, §7.2, §9.1): its head with what it does in
  // one line and *Edit* for its own fields (a Dialog L), its form as the page's first region
  // (UC-FLW-01 check 2) - a draft published whole -, where a workflow runs for it, and archiving
  // and deleting in the danger zone, deleting behind a Dialog S.
  //
  // **What the reader may never do is not drawn** (P-05): without STRUCTURE the page shows the type
  // and its use, and neither *Edit*, the form nor the danger zone.

  import { untrack } from 'svelte';
  import { DataTable, EmptyState, RecordPage, type DataColumn, type RecordList } from '@hubtask/design-system/components';
  import type { EntryType } from '@hubtask/sync-engine';

  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { hasForm } from '../lib/data/entryforms.ts';
  import { entryTypes } from '../lib/data/entrytypes.svelte.ts';
  import { workflows } from '../lib/data/workflows.svelte.ts';
  import EntryFormEditor from '../lib/entries/EntryFormEditor.svelte';
  import DeleteTypeDialog from '../lib/entrytypes/DeleteTypeDialog.svelte';
  import TypeDialog from '../lib/entrytypes/TypeDialog.svelte';
  import FailureNote from '../lib/frame/FailureNote.svelte';
  import { page } from '../lib/frame/page.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  interface Props {
    typeKey: string;
    onnavigate: (path: string) => void;
  }

  const { typeKey, onnavigate }: Props = $props();

  const LIST = '/administration/types';

  $effect(() =>
    untrack(() => {
      const closers = [entryTypes.open(), workflows.open(), containers.start()];
      return () => closers.forEach((close) => close?.());
    }),
  );

  const type = $derived(entryTypes.all.find((each) => each.key === typeKey));
  const mayShape = $derived(holdsHere('STRUCTURE') === true);
  const reading = $derived(entryTypes.state);
  const readRefusal = $derived(reading?.status === 'failed' ? renderProblem(reading.error, messages) : undefined);

  const levelName = (level: string) => t('app.entry_types.level', { level });
  const nameOf = (each: EntryType) => each.name ?? levelName(each.level);
  const name = $derived(type ? nameOf(type) : '');
  $effect(() => page.entitle(name || t('app.entry_types.title')));

  /** What the type does, in one line under its name. */
  const facts = $derived(
    type
      ? [
          levelName(type.level),
          ...(type.record ? [t('app.entry_types.record')] : []),
          ...(type.schedulable ? [] : [t('app.entry_types.not_scheduled')]),
          ...(type.workflow ? [t('app.entry_types.behaviour_workflow')] : []),
          hasForm(type.form) ? t('app.entry_types.form_set') : t('app.entry_types.form_none'),
        ].join(' · ')
      : undefined,
  );

  // Where a workflow runs for this type: the scheme's rows that name it.
  interface Use {
    readonly id: string;
    readonly place: string;
    readonly workflow: string;
  }
  const uses = $derived<Use[]>(
    (workflows.list?.scheme ?? [])
      .filter((row) => row.entry_type === typeKey)
      .map((row) => ({
        id: `${row.container_id}:${row.workflow}`,
        place: containers.find(row.container_id)?.name ?? t('app.workflow_designer.unknown_place'),
        workflow: workflows.list?.data.find((each) => each.key === row.workflow)?.name ?? t('workflows.default_name'),
      })),
  );
  const useColumns = $derived<DataColumn[]>([
    { id: 'place', label: t('app.entry_types.used_place'), priority: 1 },
    { id: 'workflow', label: t('app.entry_types.used_workflow'), priority: 2 },
  ]);

  const lists = $derived<RecordList[]>([
    ...(mayShape ? [{ id: 'form', title: t('app.entry_types.form') }] : []),
    { id: 'used', title: t('app.entry_types.used_title'), count: String(uses.length) },
  ]);

  let isEditing = $state(false);
  let isDeleting = $state(false);
  let failure = $state<string | undefined>(undefined);

  async function archive(archived: boolean) {
    if (!type) return;
    failure = undefined;
    const code = archived ? 'app.entry_types.archived_done' : 'app.entry_types.unarchived_done';
    try {
      await entryTypes.change(type, { archived });
      toast.show({ text: t(code, { name }), tone: 'success' });
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    }
  }

  const danger = $derived(
    mayShape && type && !type.system
      ? {
          title: t('app.page.danger_zone'),
          acts: [
            {
              id: 'archive',
              title: type.archived ? t('app.entry_types.unarchive') : t('app.entry_types.archive'),
              cost: type.archived ? t('app.entry_types.unarchive_cost') : t('app.entry_types.archive_cost'),
              actionLabel: type.archived ? t('app.entry_types.unarchive') : t('app.entry_types.archive'),
            },
            { id: 'delete', title: t('app.entry_types.delete', { name }), cost: t('app.entry_types.delete_explain'), actionLabel: t('app.entry_types.delete_menu') },
          ],
          onact: (id: string) => {
            if (id === 'archive') void archive(!type.archived);
            else isDeleting = true;
          },
        }
      : undefined,
  );
</script>

<RecordPage
  record={{
    kind: t('app.entry_types.kind'),
    title: name,
    back: { label: t('app.entry_types.title'), href: LIST, onnavigate: () => onnavigate(LIST) },
    status: type?.archived ? { label: t('app.entry_types.archived'), tone: 'neutral' } : type?.system ? { label: t('app.entry_types.system'), tone: 'info' } : undefined,
    facts,
    edit: mayShape && type ? { label: t('app.page.edit'), onclick: () => (isEditing = true), opener: 'edit-type' } : undefined,
  }}
  lists={type ? lists : []}
  {danger}
  refused={readRefusal || (reading?.status === 'ready' && !type) ? notHere : undefined}
>
  {#snippet list(item: RecordList)}
    {#if item.id === 'form' && type}
      {#if failure}<FailureNote problem={failure} />{/if}
      {#key type.key}<EntryFormEditor {type} {name} />{/key}
    {:else if uses.length === 0}
      <p class="quiet">{t('app.entry_types.used_nowhere')}</p>
    {:else}
      <DataTable label={item.title} columns={useColumns} rows={uses} key={(use: Use) => use.id}>
        {#snippet cell(use: Use, column: DataColumn)}
          {column.id === 'place' ? use.place : use.workflow}
        {/snippet}
      </DataTable>
    {/if}
  {/snippet}
</RecordPage>

{#snippet notHere()}
  {#if readRefusal}
    <FailureNote problem={readRefusal} />
  {:else}
    <EmptyState kind="settled" title={t('app.entry_types.gone')} />
  {/if}
{/snippet}

{#if mayShape}
  <TypeDialog type={isEditing ? type : undefined} onClose={() => (isEditing = false)} />
  <DeleteTypeDialog type={isDeleting ? type : undefined} onClose={() => (isDeleting = false)} onDeleted={() => onnavigate(LIST)} />
{/if}

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
</style>
