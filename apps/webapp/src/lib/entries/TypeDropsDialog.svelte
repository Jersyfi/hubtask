<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Confirming a change of type that drops values (UC-FLW-01 check 4): the values the new type
  // does not keep, by field, before anything is cleared. A completion is never cleared this way,
  // so where one is among them the dialog only says to reopen the entry first.

  import { Button, Dialog, Stack } from '@hubtask/design-system/components';

  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    isOpen: boolean;
    typeName: string;
    fields: readonly string[];
    isConfirmable: boolean;
    onconfirm: () => void;
  }

  let { isOpen = $bindable(false), typeName, fields, isConfirmable, onconfirm }: Props = $props();

  const fieldName = (field: string): string =>
    field === 'due_at'
      ? t('app.item.type_drop_due_at')
      : field === 'recurrence'
        ? t('app.item.type_drop_recurrence')
        : field === 'completion'
          ? t('app.item.type_drop_completion')
          : field;
</script>

<Dialog bind:isOpen title={t('app.item.type_drops_title', { type: typeName })} dismissLabel={t('app.item.type_drops_cancel')}>
  <Stack gap="200">
    {#if isConfirmable}
      <p>{t('app.item.type_drops_lead', { type: typeName })}</p>
      <ul>
        {#each fields as field (field)}
          <li>{fieldName(field)}</li>
        {/each}
      </ul>
    {:else}
      <p>{t('app.item.type_drops_reopen', { type: typeName })}</p>
    {/if}
  </Stack>
  {#snippet actions()}
    <Button tone="secondary" onclick={() => (isOpen = false)}>{t('app.item.type_drops_cancel')}</Button>
    {#if isConfirmable}
      <Button onclick={onconfirm}>{t('app.item.type_drops_confirm')}</Button>
    {/if}
  {/snippet}
</Dialog>
