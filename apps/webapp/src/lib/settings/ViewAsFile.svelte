<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *View as file* (configuration/ui.md A10): the page's part of the configuration document, in a
  // dialog - the bridge to `hubctl` and to agents. It shows what the page shows and changes
  // nothing; the document is never a second representation with values of its own.

  import { Button, CodeBlock, Dialog } from '@hubtask/design-system/components';

  import { asFile, type ShownValue } from './asFile.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    values: readonly ShownValue[];
    /** The place the page configures: `WORKSPACE`, `HUB`, … */
    place: string;
    isOpen?: boolean;
    /** The page's head draws the trigger (`viewAsFileAction`), so none is drawn here. */
    isTriggerHidden?: boolean;
  }

  let { values, place, isOpen = $bindable(false), isTriggerHidden = false }: Props = $props();
  const file = $derived(isOpen ? asFile(values, place) : '');
</script>

{#if !isTriggerHidden}
  <Button tone="subtle" size="sm" data-view-as-file onclick={() => (isOpen = true)}>{t('app.settings.view_as_file')}</Button>
{/if}

<Dialog size="l" bind:isOpen title={t('app.settings.file_title')} dismissLabel={t('app.settings.file_close')}>
  <p class="quiet">{t('app.settings.file_explain')}</p>
  <CodeBlock code={file} label={t('app.settings.file_title')} language="json" />
  {#snippet actions()}
    <Button tone="secondary" onclick={() => (isOpen = false)}>{t('app.settings.file_close')}</Button>
  {/snippet}
</Dialog>

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
</style>
