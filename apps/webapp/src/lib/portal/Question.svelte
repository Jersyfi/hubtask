<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One question of a portal form, drawn by its kind (UC-SVC-05 check 2). The field's key names
  // it, as everywhere a custom field is shown; the form's help text says what it is for.

  import { Checkbox, CheckboxGroup, Input, Select } from '@hubtask/design-system/components';

  import { t } from '../i18n/i18n.svelte.ts';
  import { controlOf, type Question } from './portal.ts';

  let { question, help, isRequired = false, value = $bindable() }: {
    question: Question; help?: string; isRequired?: boolean; value: unknown;
  } = $props();

  const control = $derived(controlOf(question));
  const options = $derived(question.options.map((option) => ({ value: option, label: option })));
  let text = $state(typeof value === 'string' ? value : '');
  let chosen = $state<string[]>(Array.isArray(value) ? (value as string[]) : []);
  let yes = $state(value === true);
  $effect(() => {
    value = control === 'choices' ? chosen : control === 'yes-no' ? yes : text;
  });
</script>

{#if control === 'choice'}
  <Select label={question.key} hint={help} {isRequired} {options} placeholder={t('app.portal.choose')} bind:value={text} />
{:else if control === 'choices'}
  <CheckboxGroup label={question.key} hint={help} {options} selected={chosen} onChange={(next) => (chosen = [...next])} />
{:else if control === 'yes-no'}
  <Checkbox label={question.key} hint={help} bind:checked={yes} />
{:else if control === 'number'}
  <Input label={question.key} hint={help} {isRequired} inputmode="decimal" bind:value={text} />
{:else if control === 'date'}
  <Input label={question.key} hint={help} {isRequired} type="date" bind:value={text} />
{:else if control === 'text'}
  <Input label={question.key} hint={help} {isRequired} bind:value={text} />
{/if}
