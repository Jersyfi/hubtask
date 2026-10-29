<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Publishing a draft (UC-FLW-02 check 4, UC-FLW-05 checks 2 and 3): first what it does - how
  // many entries move, how the board's columns change - and, per status the draft removes that
  // entries are in, where they go, from the statuses on the same side of done. Each choice asks
  // again; nothing moves until *Publish* is pressed, and then everything moves at once.

  import { untrack } from 'svelte';
  import { Banner, Button, Dialog, Select, Spinner, Stack } from '@hubtask/design-system/components';
  import type { Workflow, WorkflowPreview, WorkflowShape } from '@hubtask/sync-engine';

  import { removedOf, statusLabel, workflows } from '../data/workflows.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    workflow: Workflow;
    draft: WorkflowShape;
    onclose: () => void;
    onpublished: (published: Workflow) => void;
  }

  const { workflow, draft, onclose, onpublished }: Props = $props();

  let isOpen = $state(true);
  let mapping = $state<Record<string, string>>({});
  let preview = $state<WorkflowPreview | undefined>(undefined);
  let refusal = $state<string | undefined>(undefined);
  let isAsking = $state(false);
  let isPublishing = $state(false);

  async function ask() {
    isAsking = true;
    refusal = undefined;
    try {
      preview = await workflows.previewPublish(workflow.key, { ...mapping });
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
    } finally {
      isAsking = false;
    }
  }

  $effect(() => {
    void JSON.stringify(mapping);
    untrack(() => void ask());
  });

  const nameIn = (shape: WorkflowShape, key: string | null | undefined) => {
    const status = shape.statuses.find((each) => each.key === key);
    return status ? statusLabel(status, t) : (key ?? t('app.workflow_designer.no_status'));
  };

  /** The statuses an entry of a removed status may go to: the draft's, on the same side of done. */
  function targetsFor(removed: string) {
    const done = workflow.published.statuses.find((each) => each.key === removed)?.category === 'DONE';
    return draft.statuses
      .filter((status) => (status.category === 'DONE') === done)
      .map((status) => ({ value: status.key, label: statusLabel(status, t) }));
  }

  const removed = $derived(removedOf(preview));
  const isMapped = $derived(removed.every((each) => mapping[each.status]));

  async function publish() {
    isPublishing = true;
    refusal = undefined;
    try {
      const published = await workflows.publish(workflow, { ...mapping });
      isOpen = false;
      onpublished(published);
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
    } finally {
      isPublishing = false;
    }
  }
</script>

<Dialog
  bind:isOpen
  isWide
  title={t('app.workflow_designer.publish_title', { name: workflow.name ?? t('workflows.default_name') })}
  dismissLabel={t('app.workflow_designer.cancel')}
  onClose={onclose}
>
  <Stack gap="200">
    {#if !preview && isAsking}
      <p class="quiet"><Spinner label={t('app.workflow_designer.previewing')} /> {t('app.workflow_designer.previewing')}</p>
    {/if}
    {#if preview}
      <p data-publish-entries>{t('app.workflow_designer.publish_entries', { count: preview.entries })}</p>
      <div class="columns">
        <p class="quiet">{t('app.workflow_designer.columns_before')}</p>
        <p data-columns-before>{preview.columns_before.map((key) => nameIn(workflow.published, key)).join(' · ')}</p>
        <p class="quiet">{t('app.workflow_designer.columns_after')}</p>
        <p data-columns-after>{preview.columns_after.map((key) => nameIn(draft, key)).join(' · ')}</p>
      </div>
      {#each removed as gone (gone.status)}
        <Select
          label={t('app.workflow_designer.map_removed', { status: nameIn(workflow.published, gone.status), count: gone.count })}
          options={targetsFor(gone.status)}
          placeholder={t('app.workflow_designer.map_choose')}
          value={mapping[gone.status] ?? ''}
          data-map={gone.status}
          onchange={(event: Event) => {
            mapping = { ...mapping, [gone.status]: (event.currentTarget as HTMLSelectElement).value };
          }}
        />
      {/each}
      {#if preview.moves.length > 0}
        <ul class="moves">
          {#each preview.moves as move (`${move.collection_id}:${move.entry_type}:${move.from}`)}
            <li>{t('app.workflow_designer.move_line', {
              count: move.count, from: nameIn(workflow.published, move.from), to: nameIn(draft, move.to),
            })}</li>
          {/each}
        </ul>
      {/if}
    {/if}
    {#if refusal}<Banner tone="danger" title={refusal} />{/if}
  </Stack>
  {#snippet actions()}
    <Button tone="secondary" onclick={() => (isOpen = false)}>{t('app.workflow_designer.cancel')}</Button>
    <Button
      tone="primary" data-confirm-publish isBusy={isPublishing}
      disabledReason={!preview ? t('app.workflow_designer.previewing') : !isMapped ? t('app.workflow_designer.map_first') : undefined}
      onclick={publish}
    >{t('app.workflow_designer.publish')}</Button>
  {/snippet}
</Dialog>

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
  p { margin: 0; }
  .columns { display: grid; gap: var(--sp-050); }
  .moves { margin: 0; padding-inline-start: var(--sp-300); display: grid; gap: var(--sp-050); }
</style>
