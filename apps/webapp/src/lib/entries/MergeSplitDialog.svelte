<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Merging a request into another that asks the same, and splitting a part of one off
  // (UC-SVC-03 checks 5 and 6). A merge links the entry as the other's duplicate and archives it -
  // nothing is copied or deleted, so *Unarchive* and removing the link undo it. A split makes a new
  // entry for the same requester, linked from this one when a link type is chosen.

  import { Button, Dialog, Input, Select, Stack, Textarea } from '@hubtask/design-system/components';
  import type { WorkItem } from '@hubtask/sync-engine';

  import EntryPicker from './EntryPicker.svelte';
  import { items } from '../data/items.svelte.ts';
  import { announcer } from '../announce.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    /** What is being done, to which entry; `undefined` closes the dialog. */
    task?: { kind: 'merge' | 'split'; item: WorkItem };
    onclose: () => void;
    onopened: (itemId: string) => void;
  }

  const { task, onclose, onopened }: Props = $props();

  /** The link types a split offers: none, or one Hubtask ships whose meaning fits a part. */
  const SPLIT_LINKS = ['affects', 'implements', 'depends_on', 'blocks'] as const;

  let into = $state<WorkItem | undefined>(undefined);
  let title = $state('');
  let notes = $state('');
  let linkType = $state('');
  let isBusy = $state(false);
  let failure = $state<string | undefined>(undefined);

  $effect(() => {
    if (!task) return;
    into = undefined;
    title = '';
    notes = '';
    linkType = '';
    failure = undefined;
  });

  async function go() {
    if (!task) return;
    isBusy = true;
    failure = undefined;
    try {
      if (task.kind === 'merge') {
        if (!into) return;
        await items.merge(task.item.id, into.id);
        announcer.say(t('app.merge.done', { title: task.item.title, into: into.title }));
        onopened(into.id);
      } else {
        const result = await items.split(
          task.item.id,
          { title: title.trim(), ...(notes.trim() === '' ? {} : { notes }), ...(linkType === '' ? {} : { link_type: linkType }) },
          crypto.randomUUID(),
        );
        announcer.say(t('app.split.done', { title: title.trim() }));
        if (result.item?.id) onopened(result.item.id);
      }
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    } finally {
      isBusy = false;
    }
  }

  const ready = $derived(task?.kind === 'merge' ? into !== undefined : title.trim() !== '');
</script>

<Dialog
  isOpen={task !== undefined}
  title={task ? t(task.kind === 'merge' ? 'app.merge.of' : 'app.split.of', { title: task.item.title }) : ''}
  dismissLabel={t('app.workspace.cancel')}
  onClose={onclose}
>
  {#if task}
    <Stack gap="150">
      {#if task.kind === 'merge'}
        <p class="hint">{t('app.merge.hint')}</p>
        <EntryPicker excluded={new Set([task.item.id])} chosen={into} onchoose={(found) => (into = found)} />
      {:else}
        <p class="hint">{t('app.split.hint')}</p>
        <Input label={t('app.split.title')} bind:value={title} />
        <Textarea label={t('app.split.notes')} rows={3} bind:value={notes} />
        <Select
          label={t('app.split.link')}
          bind:value={linkType}
          placeholder={t('app.split.no_link')}
          options={SPLIT_LINKS.map((key) => ({ value: key, label: t(`app.links.out.${key}`) }))}
        />
      {/if}
      {#if failure}<p class="failure" role="alert">{failure}</p>{/if}
    </Stack>
  {/if}
  {#snippet actions()}
    <Button tone="secondary" onclick={onclose}>{t('app.workspace.cancel')}</Button>
    <Button
      tone="primary"
      data-merge-split
      isBusy={isBusy}
      busyLabel={t('app.workspace.saving')}
      disabledReason={ready ? undefined : t(task?.kind === 'merge' ? 'app.links.choose_first' : 'app.split.title_first')}
      onclick={() => void go()}
    >{t(task?.kind === 'merge' ? 'app.merge.go' : 'app.split.go')}</Button>
  {/snippet}
</Dialog>

<style>
  .hint { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .failure { margin: 0; color: var(--text-danger); }
</style>
