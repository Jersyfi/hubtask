<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One condition of a form field, picked as a field, a comparison and a value (user-experience.md
  // UX-4, §5.4); the expression it stands for is behind *Advanced*, for what a pick cannot say. An
  // expression no pick wrote opens there, and the picks say so rather than overwrite it.

  import { Disclosure, Input, Select, type SelectOption } from '@hubtask/design-system/components';

  import { conditionOf, pickOf } from '../data/entryforms.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    label: string;
    value: string;
    /** The custom fields a condition may name, with the values each offers where it has a closed set. */
    fields: readonly { readonly key: string; readonly label: string; readonly options?: readonly string[] }[];
    error?: string;
  }

  let { label, value = $bindable(), fields, error }: Props = $props();

  const pick = $derived(pickOf(value));
  let isWritten = $state(false);
  $effect.pre(() => {
    if (pick === undefined || error) isWritten = true;
  });

  const fieldOptions = $derived<SelectOption[]>([
    { value: '', label: t('app.entry_forms.condition_never') },
    ...fields.map((each) => ({ value: each.key, label: each.label })),
  ]);
  const opOptions = $derived<SelectOption[]>([
    { value: 'is', label: t('app.entry_forms.condition_is') },
    { value: 'is_not', label: t('app.entry_forms.condition_is_not') },
  ]);
  const chosen = $derived(fields.find((each) => each.key === pick?.field));

  function set(next: Partial<{ field: string; op: 'is' | 'is_not'; value: string }>) {
    const now = pick ?? { field: '', op: 'is' as const, value: '' };
    value = conditionOf({ ...now, ...next });
  }
</script>

<fieldset class="condition" data-condition={label}>
  <legend class="legend">{label}</legend>
  {#if pick}
    <div class="line">
      <Select label={t('app.entry_forms.condition_field')} size="sm" options={fieldOptions} bind:value={() => pick?.field ?? '', (next) => set({ field: next })} />
      {#if pick.field !== ''}
        <Select label={t('app.entry_forms.condition_op')} size="sm" options={opOptions} bind:value={() => pick?.op ?? 'is', (next) => set({ op: next as 'is' | 'is_not' })} />
        {#if chosen?.options && chosen.options.length > 0}
          <Select label={t('app.entry_forms.condition_value')} size="sm"
            options={[{ value: '', label: t('app.entry_forms.condition_choose') }, ...chosen.options.map((each) => ({ value: each, label: each }))]}
            bind:value={() => pick?.value ?? '', (next) => set({ value: next })} />
        {:else}
          <Input label={t('app.entry_forms.condition_value')} size="sm" bind:value={() => pick?.value ?? '', (next) => set({ value: next })} />
        {/if}
      {/if}
    </div>
  {:else}
    <p class="quiet">{t('app.entry_forms.condition_written')}</p>
  {/if}
  <Disclosure tone="quiet" label={t('app.entry_forms.condition_advanced')} bind:isOpen={isWritten}>
    <Input label={t('app.entry_forms.condition_expression', { label })} hint={t('app.entry_forms.condition_hint')} size="sm" bind:value {error} />
  </Disclosure>
</fieldset>

<style>
  .condition { display: grid; gap: var(--sp-050); margin: 0; padding: 0; border: 0; min-width: 0; }
  .legend { padding: 0; font-size: var(--fs-075); font-weight: var(--fw-medium); color: var(--text-secondary); }
  .line { display: flex; flex-wrap: wrap; gap: var(--sp-100); align-items: end; }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
</style>
