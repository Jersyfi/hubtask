<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Compare with…* as a Dialog S: the one version this one is read against.
  import { Button, Dialog, Select } from '@hubtask/design-system/components';

  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    isOpen: boolean;
    options: readonly { value: string; label: string }[];
    value: string;
    onchoose: (value: string) => void;
  }

  let { isOpen = $bindable(false), options, value, onchoose }: Props = $props();
  let choice = $state('');
  $effect(() => {
    if (isOpen) choice = value;
  });
</script>

<Dialog
  size="s"
  title={t('app.changes.compare')}
  bind:isOpen
  dismissLabel={t('app.changes.cancel')}
  primary={{ label: t('app.changes.compare_run'), onclick: () => { onchoose(choice); isOpen = false; } }}
>
  <Select label={t('app.changes.compare_from')} bind:value={choice} {options} />
  {#snippet actions()}
    <Button tone="subtle" onclick={() => (isOpen = false)}>{t('app.changes.cancel')}</Button>
  {/snippet}
</Dialog>
