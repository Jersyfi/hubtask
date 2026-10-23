<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What to do with the entries somebody picked.
  //
  // **One request, one idempotency key.** The nine operations `BulkOperation.op` declares each
  // become one operation per selected entry, sent together — because that is what the route is
  // for, and because a retry of one intent must not trash twice.
  //
  // **200 is not "it worked".** "A bulk that half succeeded is not a failed request", so the answer
  // is read per operation and reported per entry: applied, refused with its own sentence, or not
  // applied because an atomic bulk was taken back. A toast saying "failed" would be lying about
  // 499 entries.
  //
  // **`CREATE_ITEM` is the odd one and the contract says why**: it is the operation with no entry
  // yet, so it cannot be applied to a selection at all. What it can honestly mean in a bar is the
  // other bulk people actually perform — adding several entries at once, one per line — and that
  // is what it offers.
  //
  // **The cap is the installation's.** `max_bulk_operations` is in the manifest, the bar shows the
  // count against it, and Apply is off above it with the reason rather than sending a request the
  // server will refuse whole.

  import {
    AssigneeControl,
    Button,
    Checkbox,
    Select,
    Textarea,
  } from '@hubtask/design-system/components';
  import type { BulkOperation, BulkResult } from '@hubtask/sync-engine';

  import { accounts } from '../data/accounts.svelte.ts';
  import { buckets } from '../data/buckets.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { offerIn } from '../data/capability.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { items } from '../data/items.svelte.ts';
  import { labels } from '../data/labels.svelte.ts';
  import { people, type Path } from '../data/people.svelte.ts';
  import { selection } from '../data/selection.svelte.ts';
  import {
    BULK_OPERATIONS,
    createOperations,
    operationsFor,
    outcomeOf,
    tallyOf,
    type BulkOp,
  } from '../data/bulk.ts';
  import { bulkCapOf, isWithinCap } from '../data/selection.ts';
  import { announcer } from '../announce.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import { focusFirst } from '../forms/missing.ts';
  import { offerUndo, restoreOrder } from './undo.svelte.ts';

  interface Props {
    collectionId: string;
    /** The path the assignee picker offers people from — the same one the entry screen composes. */
    path: Path;
    /** What comes back, handed up so the list and the board can mark their own rows. */
    onresults: (operations: readonly BulkOperation[], results: readonly BulkResult[]) => void;
  }

  const { collectionId, path, onresults }: Props = $props();

  const cap = $derived(bulkCapOf(manifest.value?.limits as Record<string, unknown> | undefined));
  const count = $derived(selection.count);
  const overCap = $derived(!isWithinCap(count, cap));

  let op = $state<BulkOp>('COMPLETE_ITEM');
  let labelId = $state('');
  let accountId = $state('');
  let targetCollectionId = $state('');
  let bucketId = $state('');
  let titles = $state('');
  let atomic = $state(false);
  let isApplying = $state(false);
  let failure = $state<string | undefined>(undefined);
  /** *Apply* was pressed without the value its operation needs: said at that field. */
  let isValueMissing = $state(false);
  let bar = $state<HTMLElement | null>(null);

  const collectionLabels = $derived(labels.of(collectionId));
  const collectionBuckets = $derived(buckets.of(collectionId));
  /**
   * Where a move may go: the collections of the hub these entries are in.
   *
   * The hub's own level rather than every container this client has seen — the tree is read one
   * level at a time (`containers.svelte.ts`), so "all collections" is not a list this client has,
   * and inventing one from what happens to be cached would offer a different set on every screen.
   */
  const collections = $derived(path.hubId ? containers.collectionsOf(path.hubId) : []);
  /**
   * The operations offered: one that needs a value from a list is not offered while the list is
   * empty - a label to add where the collection has none - since nothing could ever be chosen.
   */
  /**
   * Whether some level the collection offers carries the capability (UC-WRK-23 check 4): a list
   * whose entries carry no labels or person is offered neither in bulk. Without a structure, the
   * installation's answer stands, as everywhere.
   */
  const structureHere = $derived(containers.find(collectionId)?.structure?.settings);
  const offer = $derived(offerIn(structureHere));
  const carried = (capability: string): boolean =>
    !structureHere?.levels || structureHere.levels.some((level) => offer.supports(level.type as string, capability).status === 'permitted');
  const offered = $derived(
    BULK_OPERATIONS.filter((each) => {
      if (each === 'ADD_LABEL' || each === 'REMOVE_LABEL') return collectionLabels.length > 0 && carried('LABELS');
      if (each === 'ASSIGN') return carried('ASSIGNMENT');
      if (each === 'UPDATE_ITEM') return collectionBuckets.length > 0 && carried('BUCKET');
      if (each === 'MOVE_ITEM') return collections.length > 0;
      return true;
    }),
  );

  const candidateIds = $derived(people.candidates(path));
  const candidates = $derived(
    candidateIds.map((id) => ({ id, name: accounts.nameOf(id) ?? t('app.people.unnamed') })),
  );

  /**
   * Whether the operation has everything it needs. *Apply* is never switched off for it
   * (design-system.md §4, case 3): pressed without it, the field says so and takes focus.
   */
  const ready = $derived.by(() => {
    switch (op) {
      case 'ADD_LABEL':
      case 'REMOVE_LABEL':
        return labelId !== '';
      case 'ASSIGN':
        return accountId !== '';
      case 'MOVE_ITEM':
        return targetCollectionId !== '';
      case 'UPDATE_ITEM':
        return bucketId !== '';
      case 'CREATE_ITEM':
        return titles.trim() !== '';
      default:
        return true;
    }
  });

  /** The payload of one operation: the body its own route takes, by the names it takes there. */
  function payloadFor(): Record<string, unknown> {
    switch (op) {
      case 'ADD_LABEL':
      case 'REMOVE_LABEL':
        return { label_id: labelId };
      case 'ASSIGN':
        return { account_id: accountId };
      case 'MOVE_ITEM':
        return { target_collection_id: targetCollectionId, target_parent_id: null };
      case 'UPDATE_ITEM':
        return { bucket_id: bucketId };
      default:
        return {};
    }
  }

  function operations(): readonly BulkOperation[] {
    // The one that is not about the selection. Everything else is one operation per picked entry.
    if (op === 'CREATE_ITEM') {
      return createOperations(titles, { collection_id: collectionId, type: 'TASK' });
    }
    return operationsFor(op, selection.ids, payloadFor());
  }

  /**
   * Applies the operation. Nothing asks first: the trash is reversible, so it is at once and offers
   * *Undo* (UC-WRK-28 check 3) - one atomic bulk that brings the selection back, ancestors first.
   */
  function apply() {
    if (!ready) {
      isValueMissing = true;
      queueMicrotask(() => focusFirst(bar));
      return;
    }
    void send();
  }

  async function send() {
    const sent = operations();
    if (sent.length === 0) return;
    // Read before the write: once in the trash an entry's depth is no longer on screen to read.
    const backOrder = op === 'TRASH_ITEM' ? restoreOrder(selection.ids) : [];
    const removedLabel = labelId;

    isApplying = true;
    failure = undefined;
    try {
      const results = await items.bulk(sent, atomic, crypto.randomUUID());
      onresults(sent, results);
      const applied = new Set(
        results.filter((result) => outcomeOf(result) === 'applied').map((result) => sent[result.index ?? -1]?.item_id),
      );
      if (op === 'TRASH_ITEM' && applied.size > 0) {
        offerUndo({ kind: 'bulk-trash', itemIds: backOrder.filter((id) => applied.has(id)) });
      } else if (op === 'REMOVE_LABEL' && applied.size > 0) {
        offerUndo({ kind: 'bulk-label-removed', itemIds: sent.map((each) => each.item_id ?? '').filter((id) => applied.has(id)), labelId: removedLabel });
      }

      const tally = tallyOf(results);
      // What happened, in one sentence, in the live region — because the rows change under
      // somebody who may not be watching them.
      announcer.say(
        t('app.bulk.report', {
          applied: String(tally.applied),
          refused: String(tally.refused),
          rolled: String(tally.not_applied),
        }),
      );
      if (tally.refused === 0 && tally.not_applied === 0) {
        selection.clear();
        titles = '';
      }
    } catch (error) {
      // A refusal of the *request* — over the cap, no permission at all, a malformed document.
      // Not a refusal of an operation, which never arrives this way.
      failure = renderProblem(error as never, messages).message;
    } finally {
      isApplying = false;
    }
  }
</script>

<!-- Drawn only while somebody is selecting (ADR-0063 decision 8). It used to stand above every
     list saying "select every entry on screen" whether or not anybody was selecting anything. -->
{#if selection.isOn}
<div class="bar" bind:this={bar} data-active={count > 0 || op === 'CREATE_ITEM' ? '' : undefined}>
  <!-- The way in, and the only control here that is drawn with nothing selected: without it a
       keyboard reader would have to press every row to act on a screenful. -->
  <Checkbox
    label={t('app.bulk.select_all')}
    checked={selection.isAllVisible}
    onclick={() => selection.allVisible()}
    onkeydown={(event: KeyboardEvent) => {
      if (event.key !== ' ' && event.key !== 'Enter') return;
      event.preventDefault();
      selection.allVisible();
    }}
  />

  {#if count > 0 || op === 'CREATE_ITEM'}
      <span class="count">
        {cap === undefined
          ? t('app.bulk.selected', { count: String(count) })
          : t('app.bulk.selected_of_cap', { count: String(count), cap: String(cap) })}
      </span>

      <Select
        label={t('app.bulk.operation')}
        size="sm"
        bind:value={op}
        options={offered.map((each) => ({ value: each, label: t(`app.bulk.op_${each}`) }))}
        onchange={() => (isValueMissing = false)}
      />

      {#if op === 'ADD_LABEL' || op === 'REMOVE_LABEL'}
        <Select
          label={t('app.bulk.label')}
          size="sm"
          bind:value={labelId}
          error={isValueMissing && labelId === '' ? t('app.bulk.needs_value') : undefined}
          onchange={() => (isValueMissing = false)}
          options={collectionLabels.map((label) => ({ value: label.id, label: label.name }))}
        />
      {:else if op === 'ASSIGN'}
        <AssigneeControl
          label={t('app.bulk.assignee')}
          {candidates}
          selected={accountId === '' ? [] : [accountId]}
          selection="single"
          filterLabel={t('app.bulk.assignee_filter')}
          locale={messages.locale}
          emptyLabel={t('app.bulk.assignee_empty')}
          noMatchLabel={t('app.bulk.assignee_no_match')}
          chosenLabel={t('app.bulk.assignee_chosen')}
          unassignedLabel={t('app.bulk.assignee_none')}
          error={isValueMissing && accountId === '' ? t('app.bulk.needs_value') : undefined}
          onSelect={(ids) => {
            accountId = ids[0] ?? '';
            isValueMissing = false;
          }}
        />
      {:else if op === 'MOVE_ITEM'}
        <Select
          label={t('app.bulk.collection')}
          size="sm"
          bind:value={targetCollectionId}
          error={isValueMissing && targetCollectionId === '' ? t('app.bulk.needs_value') : undefined}
          onchange={() => (isValueMissing = false)}
          options={collections.map((collection) => ({ value: collection.id, label: collection.name }))}
        />
      {:else if op === 'UPDATE_ITEM'}
        <Select
          label={t('app.bulk.bucket')}
          size="sm"
          bind:value={bucketId}
          error={isValueMissing && bucketId === '' ? t('app.bulk.needs_value') : undefined}
          onchange={() => (isValueMissing = false)}
          options={collectionBuckets.map((bucket) => ({ value: bucket.id, label: bucket.name }))}
        />
      {:else if op === 'CREATE_ITEM'}
        <Textarea
          label={t('app.bulk.titles')}
          bind:value={titles}
          rows={3}
          error={isValueMissing && titles.trim() === '' ? t('app.bulk.needs_value') : undefined}
          oninput={() => (isValueMissing = false)}
        />
      {/if}

      <!-- Off by default and offered where it matters: all-or-nothing is the right answer for a move
           and the wrong one for ticking off forty entries, where what applied should stay applied. -->
      <Checkbox label={t('app.bulk.atomic')} hint={t('app.bulk.atomic_hint')} bind:checked={atomic} />

      <Button
        size="sm"
        isBusy={isApplying}
        busyLabel={t('app.workspace.saving')}
        disabledReason={overCap ? t('app.bulk.over_cap') : undefined}
        onclick={apply}
      >
        {t('app.bulk.apply')}
      </Button>

      {#if count > 0}
        <Button size="sm" tone="secondary" onclick={() => selection.clear()}>
          {t('app.bulk.clear')}
        </Button>
      {/if}

      {#if failure}<p class="failure" role="alert">{failure}</p>{/if}
  {/if}

  <!-- The way out, always drawn while the mode is on: a mode with no visible end is one somebody
       is stuck in. `Escape` does the same, from anywhere on the screen. -->
  <span class="done">
    <Button size="sm" tone="subtle" icon="x" onclick={() => selection.stop()}>
      {t('app.bulk.done')}
    </Button>
  </span>
</div>
{/if}

<style>
  /* Pushed to the end of the row, because it is the way out rather than one of the verbs. */
  .done { margin-inline-start: auto; }

  /* Sticky once something is selected (ADR-0061 decision 4): the bar is about the rows below it,
     and a reader who picked forty entries should not scroll back up to act on them. Under the app
     bar from `medium`; on a phone above the bottom bar, where the thumb is. `raised`, because it
     sits over the rows it scrolls past and under every overlay. */
  .bar {
    position: sticky;
    inset-block-start: calc(var(--layout-appbar-height) + var(--sp-100));
    z-index: var(--z-raised);
    display: flex;
    flex-wrap: wrap;
    align-items: end;
    gap: var(--sp-100);
    padding: var(--sp-100);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    background: var(--bg-surface);
  }

  /* On a phone the bar sits above the rows in the flow until something is picked, and then it is
     fixed above the bottom bar: a sticky element that stands *before* what scrolls cannot stick to
     the bottom, so this is `fixed` for as long as there is a selection to act on. */
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    .bar { position: static; }

    .bar[data-active] {
      position: fixed;
      inset-inline: var(--sp-200);
      inset-block-start: auto;
      inset-block-end: calc(var(--layout-bottombar-height) + var(--sp-100) + env(safe-area-inset-bottom, 0));
      /* A sheet of controls, one under the other: a row that wraps five controls at a phone's
         width is five lines of uneven height. */
      flex-direction: column;
      align-items: stretch;
      max-block-size: 60vh;
      overflow: auto;
      /* Rule 1: raised, because it stands over the rows; not an overlay, because it is not temporary. */
      box-shadow: var(--shadow-raised);
    }
  }

  .count { align-self: center; font-size: var(--fs-075); font-weight: var(--fw-semibold); }

  .failure { margin: 0; width: 100%; color: var(--text-danger); font-size: var(--fs-075); }
</style>
