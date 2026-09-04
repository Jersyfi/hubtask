<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The entries of a collection, one level at a time.
  //
  // `LIST_COLLAPSED` and `LIST_EXPANDED` are not two components and not a prop: they are whether a
  // row's children are shown, one row at a time, and expanding is what fetches them. A layout that
  // read the whole subtree would be fetching levels nobody has opened.
  //
  // Which types may be created where comes from the **manifest** (`lib/data/capability.ts`), never
  // from the three names this product happens to have today: `domain-model.md` §2's extension
  // example — a new type is a profile entry and no code change — is only true if nothing here
  // spells them out.

  import { untrack } from 'svelte';

  import {
    Button,
    EmptyState,
    ErrorState,
    Inline,
    Input,
    Select,
    Skeleton,
    Stack,
    TaskRow,
  } from '@hubtask/design-system/components';
  import type { WorkItem } from '@hubtask/sync-engine';

  import { childTypes, rootTypes } from '../data/capability.svelte.ts';
  import { items } from '../data/items.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    collectionId: string;
    /** Whether the collection is archived — the entries in it are then read-only too (I-C3). */
    isReadOnly?: boolean;
  }

  const { collectionId, isReadOnly = false }: Props = $props();

  /** The entries whose children are shown. Expanding one is what reads its level. */
  let expanded = $state<string[]>([]);

  // `untrack` for the reason F2-08 records: the listener writes the store and writing it reads it,
  // so an effect that subscribes while tracking that read cancels itself before the answer lands.
  $effect(() => {
    const wanted = collectionId;
    return untrack(() => items.openCollection(wanted));
  });

  $effect(() => {
    const open = [...expanded];
    return untrack(() => {
      const stops = open.map((id) => items.openChildren(id));
      return () => {
        for (const stop of stops) stop();
      };
    });
  });

  interface Row {
    readonly item: WorkItem;
    readonly depth: number;
    readonly takesChildren: boolean;
  }

  /** The visible rows, in the order the eye walks them. */
  function flatten(parents: readonly WorkItem[], depth: number): Row[] {
    const rows: Row[] = [];
    for (const item of parents) {
      // Whether it *may* hold children is the manifest's answer; whether it *does* is not known
      // until it is opened, which is why the twist appears for any type that takes them.
      rows.push({ item, depth, takesChildren: childTypes(item.type).length > 0 });
      if (expanded.includes(item.id)) rows.push(...flatten(items.childrenOf(item.id), depth + 1));
    }
    return rows;
  }

  const rows = $derived(flatten(items.inCollection(collectionId), 0));
  // Not called `state`: a variable of that name collides with the `$state` rune in what the
  // compiler generates, and the error it produces names a line that looks unrelated.
  const levelState = $derived(items.stateOf(`container:${collectionId}`));
  const failure = $derived(
    levelState?.status === 'failed' ? renderProblem(levelState.error, messages) : undefined,
  );

  /** Which parent the add form is open under: an entry's id, `'root'`, or nothing. */
  let addingUnder = $state<string | null>(null);
  let draftTitle = $state('');
  let draftType = $state('');
  let isSaving = $state(false);
  let writeFailure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  const offeredTypes = $derived(
    addingUnder === null
      ? []
      : addingUnder === 'root'
        ? rootTypes()
        : childTypes(rows.find((row) => row.item.id === addingUnder)?.item.type ?? ''),
  );

  function startAdding(under: string) {
    addingUnder = under;
    draftTitle = '';
    writeFailure = undefined;
    // The first type the manifest offers, so the common case needs no choice at all.
    draftType =
      (under === 'root'
        ? rootTypes()
        : childTypes(rows.find((row) => row.item.id === under)?.item.type ?? ''))[0] ?? '';
  }

  async function create() {
    if (draftTitle.trim() === '' || draftType === '' || addingUnder === null) return;
    isSaving = true;
    writeFailure = undefined;
    try {
      await items.create(
        addingUnder === 'root'
          ? { type: draftType, collection_id: collectionId, title: draftTitle.trim() }
          : { type: draftType, parent_id: addingUnder, title: draftTitle.trim() },
        crypto.randomUUID(),
      );
      // The new entry appears because the write invalidated the level and the engine re-read it.
      // Nothing is inserted into the list by hand here.
      //
      // Expanding the parent is not decoration: a child created under a collapsed row is a row
      // nobody can see, so the reader would press Create and watch nothing happen. It opens the
      // level it was just added to, which is also what fetches it.
      if (addingUnder !== 'root' && !expanded.includes(addingUnder)) {
        expanded = [...expanded, addingUnder];
      }
      addingUnder = null;
    } catch (error) {
      writeFailure = renderProblem(error as never, messages);
    } finally {
      isSaving = false;
    }
  }

  async function toggleComplete(item: WorkItem) {
    writeFailure = undefined;
    try {
      // Re-read rather than predicted: with `completionPolicy = ROLLUP` a parent completes when its
      // children do (I-W5), and that is a change this client learns by asking.
      await items.setCompleted(item.id, !item.completion?.is_completed, crypto.randomUUID());
    } catch (error) {
      writeFailure = renderProblem(error as never, messages);
    }
  }
</script>

<Stack gap="200">
  {#if levelState === undefined || levelState.status === 'loading' || levelState.status === 'idle'}
    <div aria-busy="true"><Skeleton lines={4} /></div>
  {:else if failure}
    <ErrorState
      title={failure.message}
      reference={failure.reference}
      referenceLabel={t('app.reference')}
      retryLabel={t('app.retry')}
      onRetry={() => items.openCollection(collectionId)()}
    />
  {:else}
    {#if writeFailure}
      <p class="failure">{writeFailure.message}</p>
    {/if}

    {#if rows.length === 0}
      <!-- The form belongs here as well as below. §4.1's empty state is the only one that carries a
           call to action, and a call to action that sets a flag nothing renders is a button that
           does nothing — which is what this was until it was pressed. -->
      {#if addingUnder === 'root'}
        {@render addForm()}
      {:else}
        <EmptyState kind="unused" title={t('app.entries.none')} icon="task">
          {#snippet action()}
            {#if !isReadOnly && rootTypes().length > 0}
              <Button onclick={() => startAdding('root')}>{t('app.entries.add')}</Button>
            {/if}
          {/snippet}
        </EmptyState>
      {/if}
    {:else}
      <Stack gap="050">
        {#each rows as row (row.item.id)}
          <TaskRow
            type={row.item.type}
            title={row.item.title}
            depth={row.depth}
            isCompleted={row.item.completion?.is_completed ?? false}
            expansion={!row.takesChildren
              ? 'leaf'
              : expanded.includes(row.item.id)
                ? 'expanded'
                : 'collapsed'}
            completeLabel={t(
              row.item.completion?.is_completed ? 'app.entries.reopen' : 'app.entries.complete',
              { title: row.item.title },
            )}
            completeDisabledReason={isReadOnly ? t('app.entries.read_only') : undefined}
            expandLabel={t(
              expanded.includes(row.item.id) ? 'app.entries.collapse' : 'app.entries.expand',
              { title: row.item.title },
            )}
            onToggleComplete={() => toggleComplete(row.item)}
            onToggleExpand={() =>
              (expanded = expanded.includes(row.item.id)
                ? expanded.filter((id) => id !== row.item.id)
                : [...expanded, row.item.id])}
          >
            {#snippet trailing()}
              <!-- Offered only where the manifest permits a child. A type that takes none has no
                   control here rather than a dead one — and which types those are is read, never
                   named. -->
              {#if !isReadOnly && row.takesChildren}
                <Button
                  size="sm"
                  tone="subtle"
                  icon="plus"
                  onclick={() => startAdding(row.item.id)}
                >
                  {t('app.entries.add')}
                </Button>
              {/if}
            {/snippet}
          </TaskRow>

          {#if addingUnder === row.item.id}
            {@render addForm()}
          {/if}
        {/each}
      </Stack>

      {#if !isReadOnly && rootTypes().length > 0}
        {#if addingUnder === 'root'}
          {@render addForm()}
        {:else}
          <div>
            <Button tone="secondary" icon="plus" onclick={() => startAdding('root')}>
              {t('app.entries.add')}
            </Button>
          </div>
        {/if}
      {/if}
    {/if}
  {/if}
</Stack>

{#snippet addForm()}
  <Stack gap="150">
    <!-- The chooser appears only when there is a choice: at most installations a work package takes
         activities and nothing else, and a select with one option is a decision nobody has. -->
    {#if offeredTypes.length > 1}
      <Select
        label={t('app.entries.type_label')}
        size="sm"
        bind:value={draftType}
        options={offeredTypes.map((type) => ({ value: type, label: type }))}
      />
    {/if}
    <Input label={t('app.entries.new_title')} bind:value={draftTitle} />
    <Inline gap="100">
      <!-- Off with a reason rather than silently doing nothing. `create()` returns early on an
           empty title, and a button that answers a press with nothing is the silent ignoring this
           system has a rule against — there is no `disabled` boolean anywhere for exactly this. -->
      <Button
        size="sm"
        isBusy={isSaving}
        busyLabel={t('app.entries.creating')}
        disabledReason={draftTitle.trim() === '' ? t('app.entries.title_required') : undefined}
        onclick={create}
      >
        {t('app.entries.create')}
      </Button>
      <Button size="sm" tone="secondary" onclick={() => (addingUnder = null)}>
        {t('app.entries.cancel')}
      </Button>
    </Inline>
  </Stack>
{/snippet}

<style>
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); max-width: 64ch; }
</style>
