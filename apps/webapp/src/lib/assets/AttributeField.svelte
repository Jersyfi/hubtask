<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One typed attribute of a CI's class as a field: a choice and a yes / no as a list, a date as a
  // date, a group or an account picked by name. The text is read into its kind by `parseAttribute`,
  // so the field never decides what a value means.

  import { Input, Select, type SelectOption } from '@hubtask/design-system/components';
  import type { CiAttribute } from '@hubtask/sync-engine';

  import { messages, t } from '../i18n/i18n.svelte.ts';

  interface Props {
    attribute: CiAttribute;
    value?: string;
    groups?: readonly SelectOption[];
    accounts?: readonly SelectOption[];
  }

  let { attribute, value = $bindable(''), groups = [], accounts = [] }: Props = $props();

  const code = $derived(`app.assets.attribute.${attribute.key}`);
  const word = $derived(messages.has(code) ? t(code) : attribute.key);
  const need = $derived(attribute.need === 'REQUIRED' || attribute.need === 'RECOMMENDED' ? t(`app.assets.need.${attribute.need}`) : undefined);
  const label = $derived(need ? t('app.assets.attribute_need', { name: word, need }) : word);
  const unset = $derived({ value: '', label: t('app.assets.unset') });
</script>

{#if attribute.kind === 'CHOICE'}
  <Select {label} options={[unset, ...(attribute.options ?? []).map((each) => ({ value: each, label: each }))]} bind:value isRequired={attribute.need === 'REQUIRED'} />
{:else if attribute.kind === 'BOOLEAN'}
  <Select {label} options={[unset, { value: 'true', label: t('app.assets.yes') }, { value: 'false', label: t('app.assets.no') }]} bind:value />
{:else if attribute.kind === 'GROUP'}
  <Select {label} options={[unset, ...groups]} bind:value isRequired={attribute.need === 'REQUIRED'} />
{:else if attribute.kind === 'ACCOUNT'}
  <Select {label} options={[unset, ...accounts]} bind:value isRequired={attribute.need === 'REQUIRED'} />
{:else if attribute.kind === 'DATE'}
  <Input {label} type="date" bind:value isRequired={attribute.need === 'REQUIRED'} />
{:else if attribute.kind === 'NUMBER'}
  <Input {label} inputmode="decimal" bind:value isRequired={attribute.need === 'REQUIRED'} />
{:else}
  <Input {label} type={attribute.kind === 'URL' ? 'url' : 'text'} bind:value isRequired={attribute.need === 'REQUIRED'} />
{/if}
