<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Creating a column, and changing one.
  //
  // One form for both, because a column is three fields and they are the same three whichever way
  // you arrive at them: what it is called, how many entries it is meant to hold, and whether an
  // entry moved into it counts as done. Two forms would be two places to add the fourth.
  //
  // The limit and the done flag are the reason this is a form rather than a rename. The board
  // already reads both — it draws the over-limit warning and the done-column note — and until now
  // neither could be set anywhere but with curl.

  import { Button, Dialog, Input, Stack, Switch } from '@hubtask/design-system/components';

  import { buckets } from '../data/buckets.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  import type { Bucket } from '@hubtask/sync-engine';

  interface Props {
    isOpen: boolean;
    collectionId: string;
    /** The column being changed. Absent creates one. */
    bucket?: Bucket;
  }

  let { isOpen = $bindable(), collectionId, bucket }: Props = $props();

  let name = $state('');
  let limit = $state('');
  let isDone = $state(false);
  let isSaving = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  // Opening fills the form from the column, or empties it for a new one. Closing leaves it alone:
  // the fields are re-read on the next open, and clearing them here would blank the dialog while
  // it is still fading out.
  $effect(() => {
    if (!isOpen) return;
    name = bucket?.name ?? '';
    limit = bucket?.wip_limit != null ? String(bucket.wip_limit) : '';
    isDone = bucket?.is_done_bucket ?? false;
    failure = undefined;
  });

  /**
   * The limit as the contract wants it: a number, or null for "no limit".
   *
   * A string field rather than a number input, so that clearing it is a thing a reader can do. An
   * empty number input and a zero are the same keystrokes away from each other, and only one of
   * them means "stop warning me".
   */
  const parsedLimit = $derived.by(() => {
    const text = limit.trim();
    if (text === '') return null;
    const value = Number(text);
    return Number.isInteger(value) && value > 0 ? value : undefined;
  });

  const limitProblem = $derived(parsedLimit === undefined ? t('app.board.limit_invalid') : undefined);

  async function save() {
    if (name.trim() === '' || parsedLimit === undefined || isSaving) return;
    isSaving = true;
    failure = undefined;
    try {
      if (bucket) {
        // The version the reader had when they opened the form, so a change that lost a race is
        // refused rather than winning by being second (ADR-0025).
        await buckets.update(
          collectionId,
          bucket.id,
          { name: name.trim(), wip_limit: parsedLimit, is_done_bucket: isDone },
          bucket.version,
        );
      } else {
        await buckets.create(
          collectionId,
          { name: name.trim(), wip_limit: parsedLimit, is_done_bucket: isDone },
          crypto.randomUUID(),
        );
      }
      isOpen = false;
    } catch (error) {
      failure = renderProblem(error as never, messages);
    } finally {
      isSaving = false;
    }
  }
</script>

<Dialog
  bind:isOpen
  title={t(bucket ? 'app.board.edit_column' : 'app.board.create_column')}
  dismissLabel={t('app.workspace.cancel')}
>
  <Stack gap="150">
    <Input label={t('app.board.column_name')} bind:value={name} isRequired />
    <Input
      label={t('app.board.wip_limit')}
      hint={t('app.board.wip_limit_hint')}
      bind:value={limit}
      error={limitProblem}
    />
    <Switch label={t('app.board.is_done_column')} hint={t('app.board.done_bucket')} bind:checked={isDone} />
    {#if failure}<p class="failure">{failure.message}</p>{/if}
  </Stack>

  {#snippet actions()}
    <Button tone="secondary" onclick={() => (isOpen = false)}>{t('app.workspace.cancel')}</Button>
    <Button
      isBusy={isSaving}
      busyLabel={t('app.workspace.saving')}
      disabledReason={name.trim() === ''
        ? t('app.board.column_name_required')
        : (limitProblem ?? undefined)}
      onclick={save}
    >
      {t('app.workspace.save')}
    </Button>
  {/snippet}
</Dialog>

<style>
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); max-width: 64ch; }
</style>
