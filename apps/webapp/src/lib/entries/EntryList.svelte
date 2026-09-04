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
    IconButton,
    Inline,
    Input,
    LabelChip,
    LabelPicker,
    Menu,
    Popover,
    Select,
    Skeleton,
    Stack,
    TaskRow,
    rankIntent,
    rankTarget,
    type RankCommand,
  } from '@hubtask/design-system/components';
  import type { WorkItem } from '@hubtask/sync-engine';

  import { announcer } from '../announce.svelte.ts';
  import { childTypes, rootTypes, supports } from '../data/capability.svelte.ts';
  import { items } from '../data/items.svelte.ts';
  import { labels } from '../data/labels.svelte.ts';
  import { anchorFor } from '../data/rank.ts';
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

  // The collection's labels, read once for the whole list: every row picks from the same set,
  // because a label belongs to a collection (I-W3).
  $effect(() => {
    const wanted = collectionId;
    return untrack(() => labels.open(wanted));
  });

  const available = $derived(
    labels.of(collectionId).map((entry) => ({
      id: entry.id,
      name: entry.name,
      colorToken: entry.color_token,
      description: entry.description,
    })),
  );

  async function toggleLabel(item: WorkItem, labelId: string) {
    writeFailure = undefined;
    try {
      const isOn = (item.label_ids ?? []).includes(labelId);
      await labels.setOnItem(item.id, labelId, !isOn, crypto.randomUUID());
    } catch (error) {
      writeFailure = renderProblem(error as never, messages);
    }
  }

  interface Row {
    readonly item: WorkItem;
    readonly depth: number;
    readonly takesChildren: boolean;
    /**
     * The level this row is ranked within, and where it sits in it.
     *
     * Carried on the row rather than looked up when a command runs, because the flattened list is
     * not the level: a row at depth 2 is ranked among its parent's children, and the rows above it
     * on screen belong to other levels. `:reorder` moves an entry within its own level and nowhere
     * else, so the level is what a command has to be answered against.
     */
    readonly siblings: readonly WorkItem[];
    readonly index: number;
  }

  /** The visible rows, in the order the eye walks them. */
  function flatten(parents: readonly WorkItem[], depth: number): Row[] {
    const rows: Row[] = [];
    parents.forEach((item, index) => {
      // Whether it *may* hold children is the manifest's answer; whether it *does* is not known
      // until it is opened, which is why the twist appears for any type that takes them.
      rows.push({
        item,
        depth,
        takesChildren: childTypes(item.type).length > 0,
        siblings: parents,
        index,
      });
      if (expanded.includes(item.id)) rows.push(...flatten(items.childrenOf(item.id), depth + 1));
    });
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

  /**
   * Ranks a row within its level — the whole command path, in one function.
   *
   * **Built first, and the pointer path is measured against it.** WCAG 2.2 SC 2.5.7 wants a
   * single-pointer alternative to every dragging movement, and a rank change is a command before
   * it is a gesture: a menu item and a key press are this call, and so is a drag. Built the other
   * way round it would be a retrofit, and the retrofit is what an audit finds.
   *
   * The `Idempotency-Key` is minted **per intent**. Pressing "move down" twice is two intents and
   * moves the entry twice, which is what the reader asked for; one press delivered twice — a proxy
   * retry, a flaky connection — is one key and the server replays its own answer rather than
   * moving anything a second time.
   *
   * Nothing is inserted into the list by hand. The write invalidates `/items` and the level is
   * read again, so the order on screen is the server's order rather than this component's guess at
   * it — which is also what makes a second client's move appear rather than being overwritten.
   */
  async function rank(row: Row, command: RankCommand) {
    const target = rankTarget(command, row.index, row.siblings.length);
    if (target === null) return;

    writeFailure = undefined;
    try {
      await items.reorder(
        row.item.id,
        anchorFor(row.siblings, row.item.id, target),
        row.item.version,
        crypto.randomUUID(),
      );
      // Said out loud, because a rank change is invisible to a screen reader: the focused control
      // did not change and neither did its name, and the list quietly rearranged itself.
      announcer.say(
        t('app.rank.announced', {
          title: row.item.title,
          position: target + 1,
          count: row.siblings.length,
        }),
      );
    } catch (error) {
      writeFailure = renderProblem(error as never, messages);
    }
  }

  /** The reason a command is unavailable, or nothing. There is no `disabled` boolean anywhere. */
  function rankReason(row: Row, command: RankCommand): string | undefined {
    if (isReadOnly) return t('app.entries.read_only');
    if (rankTarget(command, row.index, row.siblings.length) !== null) return undefined;
    return command === 'up' || command === 'top'
      ? t('app.rank.already_first')
      : t('app.rank.already_last');
  }

  /**
   * The keyboard shortcuts, on the level rather than on a row.
   *
   * Listened for rather than bound in the markup: a `<div>` with an `onkeydown` is a static element
   * with an interaction, which Svelte warns about and is right to — the handler here belongs to the
   * *list*, and the events reach it from the row controls that are properly focusable. Attaching it
   * in an effect also means it is removed when the list is, with nothing for a caller to forget.
   */
  let level = $state<HTMLElement | null>(null);

  $effect(() => {
    const node = level;
    if (!node) return;

    const onKeydown = (event: KeyboardEvent) => {
      const id = (event.target as Element | null)?.closest?.('[data-row]')?.getAttribute('data-row');
      const row = rows.find((each) => each.item.id === id);
      if (!row || isReadOnly) return;

      const target = rankIntent(event, row.index, row.siblings.length);
      if (target === null) return;
      // Alt+Down scrolls in some engines. The command is answered, so the scroll is not.
      event.preventDefault();
      void rank(row, keyCommand(event));
    };

    node.addEventListener('keydown', onKeydown);
    return () => node.removeEventListener('keydown', onKeydown);
  });

  /** Which command a press was, once `rankIntent` has said it is one at all. */
  function keyCommand(event: KeyboardEvent): RankCommand {
    if (event.key === 'ArrowUp') return event.shiftKey ? 'top' : 'up';
    return event.shiftKey ? 'bottom' : 'down';
  }

  const RANK_COMMANDS: readonly { readonly id: RankCommand; readonly label: string }[] = [
    { id: 'up', label: 'app.rank.up' },
    { id: 'down', label: 'app.rank.down' },
    { id: 'top', label: 'app.rank.top' },
    { id: 'bottom', label: 'app.rank.bottom' },
  ];

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
      <!-- The level, and the element the shortcuts are listened for on. `data-row` is what a key
           press finds its way back from the focused control to the row by; the handler is added in
           an effect rather than bound here, because a `<div>` with an `onkeydown` is a static
           element with an interaction and Svelte is right to warn about one. -->
      <div class="level" bind:this={level}>
        {#each rows as row (row.item.id)}
          <div class="row" data-row={row.item.id}>
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
                <!-- The labels this entry carries, and a way to change them — but only for a type
                     whose profile has LABELS. The manifest answers that; an ACTIVITY has none, and
                     §2 says a field whose capability is off is refused rather than ignored, so it is
                     not offered here at all. -->
                {#if supports(row.item.type, 'LABELS').status === 'permitted'}
                  {#each (row.item.label_ids ?? []) as labelId (labelId)}
                    {@const entry = available.find((each) => each.id === labelId)}
                    {#if entry}
                      <LabelChip
                        name={entry.name}
                        colorToken={entry.colorToken}
                        description={entry.description}
                        removeLabel={isReadOnly
                          ? undefined
                          : t('app.labels.remove_from', { name: entry.name, title: row.item.title })}
                        onRemove={() => toggleLabel(row.item, labelId)}
                      />
                    {/if}
                  {/each}

                  {#if !isReadOnly}
                    <Popover label={t('app.labels.on_entry', { title: row.item.title })}>
                      {#snippet trigger(props)}
                        <Button size="sm" tone="subtle" icon="tag" {...props}>
                          {t('app.labels.choose')}
                        </Button>
                      {/snippet}
                      <LabelPicker
                        label={t('app.labels.on_entry', { title: row.item.title })}
                        labels={available}
                        selected={row.item.label_ids ?? []}
                        filterLabel={t('app.labels.filter')}
                        emptyLabel={t('app.labels.none_yet')}
                        noMatchLabel={t('app.labels.no_match')}
                        onToggle={(labelId) => toggleLabel(row.item, labelId)}
                      />
                    </Popover>
                  {/if}
                {/if}

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

                <!-- The single-pointer alternative SC 2.5.7 asks for, and the path that was built
                     first: every position a drag can reach is an item here, each with the reason it
                     cannot be used rather than a control that goes grey for no stated cause. -->
                <Menu
                  label={t('app.rank.actions', { title: row.item.title })}
                  items={RANK_COMMANDS.map((command) => ({
                    id: command.id,
                    label: t(command.label),
                    disabledReason: rankReason(row, command.id),
                  }))}
                  onselect={(id) => rank(row, id as RankCommand)}
                >
                  {#snippet trigger(props)}
                    <IconButton
                      icon="ellipsis"
                      label={t('app.rank.actions', { title: row.item.title })}
                      size="sm"
                      {...props}
                    />
                  {/snippet}
                </Menu>
              {/snippet}
            </TaskRow>

          </div>

          {#if addingUnder === row.item.id}
            {@render addForm()}
          {/if}
        {/each}
      </div>

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
  /* What `Stack gap="050"` was, written here because the level now owns a state of its own: a row
     being dragged is drawn differently, and a primitive that decorated would stop being one. */
  .level { display: flex; flex-direction: column; gap: var(--sp-050); }

  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); max-width: 64ch; }
</style>
