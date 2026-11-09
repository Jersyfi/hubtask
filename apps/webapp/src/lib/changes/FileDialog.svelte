<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *View as file* as a Dialog L: the configuration file every door reads (configuration.md §5),
  // never a second representation.
  import { Button, CodeBlock, Dialog, Spinner, Stack } from '@hubtask/design-system/components';
  import type { ConfigExport, ResourceState } from '@hubtask/sync-engine';

  import FailureNote from '../frame/FailureNote.svelte';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    isOpen: boolean;
    current: ResourceState<ConfigExport> | undefined;
  }

  let { isOpen = $bindable(false), current }: Props = $props();

  const file = $derived(current?.status === 'ready' ? JSON.stringify(current.data.document, null, 2) : '');
</script>

<Dialog size="l" title={t('app.changes.file')} bind:isOpen dismissLabel={t('app.page.close')}>
  <Stack gap="150">
    <p class="quiet" data-file>{t('app.changes.file_explain')}</p>
    {#if current?.status === 'ready'}
      <CodeBlock code={file} label={t('app.changes.file')} language="json" copyLabel={t('app.apps.copy')} copiedLabel={t('app.apps.copied')} />
    {:else if current?.status === 'failed'}
      <FailureNote problem={renderProblem(current.error, messages)} />
    {:else}
      <Spinner label={t('app.changes.reading')} />
    {/if}
  </Stack>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => (isOpen = false)}>{t('app.page.close')}</Button>
  {/snippet}
</Dialog>

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
</style>
