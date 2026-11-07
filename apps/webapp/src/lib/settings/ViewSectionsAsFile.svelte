<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *View as file* on a page whose blocks are sections of the workspace's configuration document
  // (configuration/ui.md A10): those sections of the document in force, read from `/config` - the
  // export every door answers - so the file is never a second representation. Drawn only for a
  // reader who may read the configuration (P-05).

  import { untrack } from 'svelte';
  import { Button, CodeBlock, Dialog, Spinner } from '@hubtask/design-system/components';
  import FailureNote from '../frame/FailureNote.svelte';
  import type { ConfigExport, ResourceState } from '@hubtask/sync-engine';

  import { holdsHere } from '../data/capability.svelte.ts';
  import { config } from '../data/config.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import { pagePartFile } from './asFile.ts';

  interface Props {
    /** The document's sections the page configures: `types`, `roles`, … */
    sections: readonly string[];
    /**
     * A hub whose own document the page configures. Its settings are drawn only for those who
     * shape it, so the page's own gate stands for this one.
     */
    hubId?: string;
    /** Areas of the `settings` section the page writes: `sign_in`, `time`, … */
    settings?: readonly string[];
    /** On a collection's page, its handle: only its blocks of the hub's collection sections. */
    handle?: string;
  }

  const { sections, hubId, settings, handle }: Props = $props();

  const mayRead = $derived(hubId !== undefined || holdsHere('STRUCTURE') === true || holdsHere('READ_CONFIGURATION') === true);
  let isOpen = $state(false);
  let current = $state<ResourceState<ConfigExport>>();
  $effect(() => {
    if (!isOpen) return;
    const hub = hubId;
    return untrack(() => config.document((next) => (current = next), hub));
  });

  const file = $derived.by(() => {
    if (current?.status !== 'ready') return '';
    return pagePartFile(current.data.document as Record<string, unknown>, { sections, settings, handle });
  });
</script>

{#if mayRead}
  <Button tone="subtle" size="sm" data-view-as-file onclick={() => (isOpen = true)}>{t('app.settings.view_as_file')}</Button>

  <Dialog bind:isOpen title={t('app.settings.file_title')} dismissLabel={t('app.settings.file_close')}>
    <p class="quiet">{t('app.settings.sections_file_explain')}</p>
    {#if current?.status === 'ready'}
      <CodeBlock code={file} label={t('app.settings.file_title')} language="json" />
    {:else if current?.status === 'failed'}
      <FailureNote problem={renderProblem(current.error, messages)} />
    {:else}
      <Spinner label={t('app.changes.reading')} />
    {/if}
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isOpen = false)}>{t('app.settings.file_close')}</Button>
    {/snippet}
  </Dialog>
{/if}

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
</style>
