<!-- SPDX-License-Identifier: BUSL-1.1
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
    Dialog,
    Inline,
    Select,
    Stack,
    Textarea,
  } from '@hubtask/design-system/components';
  import type { BulkOperation, BulkResult } from '@hubtask/sync-engine';

  import { accounts } from '../data/accounts.svelte.ts';
  import { buckets } from '../data/buckets.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { items } from '../data/items.svelte.ts';
  import { labels } from '../data/labels.svelte.ts';
  import { people, type Path } from '../data/people.svelte.ts';
  import { selection } from '../data/selection.svelte.ts';
  import {
    BULK_OPERATIONS,
    createOperations,
    isDestructive,
    operationsFor,
    tallyOf,
    type BulkOp,
  } from '../data/bulk.ts';
  import { bulkCapOf, isWithinCap } from '../data/selection.ts';
  import { announcer } from '../announce.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

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
  let confirming = $state(false);
  let failure = $state<string | undefined>(undefined);

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

  const candidateIds = $derived(people.candidates(path));
  const candidates = $derived(
    candidateIds.map((id) => ({ id, name: accounts.nameOf(id) ?? t('app.people.unnamed') })),
  );

  /** Whether the operation has everything it needs. Apply is off with a reason until it does. */
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

  function apply() {
    if (isDestructive(op)) {
      confirming = true;
      return;
    }
    void send();
  }

  async function send() {
    const sent = operations();
    if (sent.length === 0) return;

    isApplying = true;
    failure = undefined;
    confirming = false;
    try {
      const results = await items.bulk(sent, atomic, crypto.randomUUID());
      onresults(sent, results);

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

<div class="bar">
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
        options={BULK_OPERATIONS.map((each) => ({ value: each, label: t(`app.bulk.op_${each}`) }))}
      />

      {#if op === 'ADD_LABEL' || op === 'REMOVE_LABEL'}
        <Select
          label={t('app.bulk.label')}
          size="sm"
          bind:value={labelId}
          options={collectionLabels.map((label) => ({ value: label.id, label: label.name }))}
        />
      {:else if op === 'ASSIGN'}
        <AssigneeControl
          label={t('app.bulk.assignee')}
          {candidates}
          selected={accountId === '' ? [] : [accountId]}
          selection="single"
          filterLabel={t('app.bulk.assignee_filter')}
          emptyLabel={t('app.bulk.assignee_empty')}
          noMatchLabel={t('app.bulk.assignee_no_match')}
          chosenLabel={t('app.bulk.assignee_chosen')}
          unassignedLabel={t('app.bulk.assignee_none')}
          onSelect={(ids) => (accountId = ids[0] ?? '')}
        />
      {:else if op === 'MOVE_ITEM'}
        <Select
          label={t('app.bulk.collection')}
          size="sm"
          bind:value={targetCollectionId}
          options={collections.map((collection) => ({ value: collection.id, label: collection.name }))}
        />
      {:else if op === 'UPDATE_ITEM'}
        <Select
          label={t('app.bulk.bucket')}
          size="sm"
          bind:value={bucketId}
          options={collectionBuckets.map((bucket) => ({ value: bucket.id, label: bucket.name }))}
        />
      {:else if op === 'CREATE_ITEM'}
        <Textarea label={t('app.bulk.titles')} bind:value={titles} rows={3} />
      {/if}

      <!-- Off by default and offered where it matters: all-or-nothing is the right answer for a move
           and the wrong one for ticking off forty entries, where what applied should stay applied. -->
      <Checkbox label={t('app.bulk.atomic')} hint={t('app.bulk.atomic_hint')} bind:checked={atomic} />

      <Button
        size="sm"
        isBusy={isApplying}
        busyLabel={t('app.workspace.saving')}
        disabledReason={overCap
          ? t('app.bulk.over_cap')
          : ready
            ? undefined
            : t('app.bulk.needs_value')}
        onclick={apply}
      >
        {t('app.bulk.apply')}
      </Button>

      {#if count > 0}
        <Button size="sm" tone="secondary" onclick={() => selection.clear()}>
          {t('app.bulk.clear')}
        </Button>
      {/if}

      {#if failure}<p class="failure">{failure}</p>{/if}
  {/if}
</div>

<Dialog
  bind:isOpen={confirming}
  title={t('app.bulk.trash_title', { count: String(count) })}
  dismissLabel={t('app.workspace.cancel')}
>
  <Stack gap="150">
    <p class="hint">{t('app.bulk.trash_explains')}</p>
    <Inline gap="100">
      <Button tone="danger" isBusy={isApplying} busyLabel={t('app.workspace.saving')} onclick={() => void send()}>
        {t('app.bulk.op_TRASH_ITEM')}
      </Button>
      <Button tone="secondary" onclick={() => (confirming = false)}>{t('app.workspace.cancel')}</Button>
    </Inline>
  </Stack>
</Dialog>

<style>
  .bar {
    display: flex;
    flex-wrap: wrap;
    align-items: end;
    gap: var(--sp-100);
    padding: var(--sp-100);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    background: var(--bg-surface);
  }

  .count { align-self: center; font-size: var(--fs-075); font-weight: var(--fw-semibold); }

  .hint { margin: 0; color: var(--text-secondary); max-width: 64ch; }

  .failure { margin: 0; width: 100%; color: var(--text-danger); font-size: var(--fs-075); }
</style>
