<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *View as file* on a page whose blocks are sections of the workspace's configuration document
  // (configuration/ui.md A10): those sections of the document in force, read from `/config` - the
  // export every door answers - so the file is never a second representation. Drawn only for a
  // reader who may read the configuration (P-05).

  import { untrack } from 'svelte';
  import { Banner, Button, CodeBlock, Dialog, Spinner } from '@hubtask/design-system/components';
  import type { ConfigExport, ResourceState } from '@hubtask/sync-engine';

  import { holdsHere } from '../data/capability.svelte.ts';
  import { config } from '../data/config.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    /** The document's sections the page configures: `types`, `roles`, … */
    sections: readonly string[];
  }

  const { sections }: Props = $props();

  const mayRead = $derived(holdsHere('STRUCTURE') === true || holdsHere('READ_CONFIGURATION') === true);
  let isOpen = $state(false);
  let current = $state<ResourceState<ConfigExport>>();
  $effect(() => {
    if (!isOpen) return;
    return untrack(() => config.document((next) => (current = next)));
  });

  const file = $derived.by(() => {
    if (current?.status !== 'ready') return '';
    const document = current.data.document as Record<string, unknown>;
    return JSON.stringify(Object.fromEntries(sections.map((name) => [name, document[name] ?? {}])), null, 2);
  });
</script>

{#if mayRead}
  <Button tone="subtle" size="sm" data-view-as-file onclick={() => (isOpen = true)}>{t('app.settings.view_as_file')}</Button>

  <Dialog bind:isOpen title={t('app.settings.file_title')} dismissLabel={t('app.settings.file_close')}>
    <p class="quiet">{t('app.settings.sections_file_explain')}</p>
    {#if current?.status === 'ready'}
      <CodeBlock code={file} label={t('app.settings.file_title')} language="json" />
    {:else if current?.status === 'failed'}
      <Banner tone="danger" title={renderProblem(current.error, messages).message} />
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
