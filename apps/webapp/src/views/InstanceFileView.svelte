<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Configuration file* at the installation (user-experience.md §8.4): the installation's level
  // as the document `hubctl` and agents read - every value in force, read-only, with where it comes
  // from. In enforce mode the file on the server is the source, and the screen says which file.

  import { untrack } from 'svelte';

  import { CodeBlock, PageHeader, ReadOnlyNotice, Spinner, Stack } from '@hubtask/design-system/components';

  import InstanceGate from '../lib/instance/InstanceGate.svelte';
  import { instance } from '../lib/data/instance.svelte.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { installationTrail } from '../lib/instance/trail.ts';
  import { asFile } from '../lib/settings/asFile.ts';
  import { installationFile } from '../lib/instance/values.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import FailureNote from '../lib/frame/FailureNote.svelte';

  const { onnavigate }: { onnavigate: (path: string) => void } = $props();

  $effect(() => {
    if (!instance.isElevated) return;
    return untrack(() => instance.openSettings());
  });
  const reading = $derived(instance.settings);
  const settings = $derived(reading.status === 'ready' ? reading.data : undefined);
  const unreadable = $derived(reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined);
  const file = $derived(settings ? asFile(installationFile(settings, manifest.isSingleTenant), 'INSTANCE') : '');

  $effect(() => page.entitle(t('app.instance.file')));
</script>

<Stack gap="300">
  <PageHeader title={t('app.instance.file')} isTitleInBar={viewport.isCompact} breadcrumb={installationTrail({ id: 'screen', label: t('app.instance.file') })} />
  <InstanceGate onleave={() => onnavigate('/')}>
    <Stack gap="200">
      <p class="prose">{t('app.instance.file_intro')}</p>
      {#if unreadable}
        <FailureNote problem={unreadable} />
      {:else if !settings}
        <p class="quiet"><Spinner label={t('app.instance.reading')} /><span>{t('app.instance.reading')}</span></p>
      {:else}
        {#if settings.is_enforced_from_file}
          <ReadOnlyNotice text={`${t('app.instance.source_file_enforced')} ${settings.source ?? ''}`} />
        {/if}
        <div data-instance-file><CodeBlock code={file} label={t('app.instance.file')} language="json" /></div>
      {/if}
    </Stack>
  </InstanceGate>
</Stack>

<style>
  .prose { margin: 0; max-inline-size: 60ch; }
  .quiet { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }
</style>
