<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Copying an entry, with or without what is under it.
  //
  // **The title is the caller's.** The server copies the original's unchanged, and says why: one
  // that invented "Copy of …" would be writing display text (ADR-0011). So the field is here, empty
  // means "keep the original's", and the client offers no default of its own either — a client that
  // pre-filled "Copy of …" would be writing the same sentence one layer up.
  //
  // **What the destination cannot resolve is reported, never dropped in silence** (I-W6). A copy
  // into another collection leaves behind the labels of the vocabulary it came from, the column of
  // the board it sat on, and the values of custom fields that collection does not define — and the
  // report is the move's own, code for code, because it is the same fact.
  //
  // **The copy is opened.** It is a new entry somebody just made, and leaving them on the list to
  // find it would be the one thing they did not ask for.

  import { Button, Checkbox, Dialog, Inline, Input, Select, Stack } from '@hubtask/design-system/components';
  import type { DroppedReference, WorkItem } from '@hubtask/sync-engine';

  import { containers } from '../data/containers.svelte.ts';
  import { items } from '../data/items.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    /** The entry being copied. Present opens the dialog; `undefined` closes it. */
    item?: WorkItem;
    /** The hub whose collections a copy may go into. */
    hubId?: string;
    onclose: () => void;
    onopened: (itemId: string) => void;
  }

  const { item, hubId, onclose, onopened }: Props = $props();

  let title = $state('');
  let includeSubtree = $state(false);
  let targetCollectionId = $state('');
  let isCopying = $state(false);
  let failure = $state<string | undefined>(undefined);
  let dropped = $state<readonly DroppedReference[]>([]);
  let copied = $state<number | undefined>(undefined);
  let copyId = $state<string | undefined>(undefined);

  const collections = $derived(hubId ? containers.collectionsOf(hubId) : []);

  // Opening starts from nothing; the report of the last copy is not the answer to this one.
  $effect(() => {
    if (!item) return;
    title = '';
    includeSubtree = false;
    targetCollectionId = '';
    failure = undefined;
    dropped = [];
    copied = undefined;
    copyId = undefined;
  });

  async function duplicate() {
    if (!item) return;
    isCopying = true;
    failure = undefined;
    try {
      const result = await items.duplicate(
        item.id,
        {
          include_subtree: includeSubtree,
          // Omitted means beside the original, which is what an empty choice means here too.
          ...(targetCollectionId === ''
            ? {}
            : { target_collection_id: targetCollectionId, target_parent_id: null }),
          ...(title.trim() === '' ? {} : { title: title.trim() }),
        },
        crypto.randomUUID(),
      );
      dropped = result.dropped_references ?? [];
      copied = result.copied;
      copyId = result.item?.id;
      // Nothing was left behind, so there is nothing to read: go to the copy at once. With a report
      // the reader is shown it first and opens the copy themselves.
      if (dropped.length === 0 && copyId) onopened(copyId);
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    } finally {
      isCopying = false;
    }
  }
</script>

<Dialog
  isOpen={item !== undefined}
  title={item ? t('app.duplicate.of', { title: item.title }) : t('app.duplicate.title')}
  dismissLabel={t('app.workspace.cancel')}
  onClose={onclose}
>
  <Stack gap="150">
    <Input label={t('app.duplicate.new_title')} hint={t('app.duplicate.title_hint')} bind:value={title} />
    <Checkbox label={t('app.duplicate.subtree')} bind:checked={includeSubtree} />
    <Select
      label={t('app.duplicate.collection')}
      bind:value={targetCollectionId}
      placeholder={t('app.duplicate.here')}
      options={collections.map((collection) => ({ value: collection.id, label: collection.name }))}
    />

    {#if failure}<p class="failure">{failure}</p>{/if}

    {#if copied !== undefined}
      <p class="hint">{t('app.duplicate.copied', { count: String(copied) })}</p>
    {/if}

    {#if dropped.length > 0 && item}
      <div class="dropped">
        <p>{t('app.duplicate.left_behind', { title: item.title })}</p>
        <ul>
          {#each dropped as reference, index (`${reference.kind}-${reference.id}-${index}`)}
            <li>
              <!-- The move's own codes, kind for kind: it is the same fact reported by a different
                   operation, and a second set of sentences for it would be a second answer. -->
              <strong>{t(`app.move.dropped_kind.${reference.kind}`)}</strong>
              {t(reference.code)}
            </li>
          {/each}
        </ul>
      </div>
    {/if}

    <Inline gap="100">
      {#if copyId}
        <Button onclick={() => onopened(copyId!)}>{t('app.duplicate.open')}</Button>
      {:else}
        <Button isBusy={isCopying} busyLabel={t('app.workspace.saving')} onclick={() => void duplicate()}>
          {t('app.duplicate.go')}
        </Button>
      {/if}
      <Button tone="secondary" onclick={onclose}>{t('app.workspace.cancel')}</Button>
    </Inline>
  </Stack>
</Dialog>

<style>
  .hint { margin: 0; color: var(--text-secondary); }

  .dropped { color: var(--text-secondary); font-size: var(--fs-075); }

  .dropped ul { margin: var(--sp-050) 0 0; padding-left: var(--sp-200); }

  .failure { margin: 0; color: var(--text-danger); }
</style>
