<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One sentence of a condition: a subject, the operator it takes, and the value where there is
  // one (decision 3). The row is what the tree composer repeats; it knows nothing of the tree.

  import { Input, Select } from '@hubtask/design-system/components';

  import { OPERATORS, takes, type Sentence, type Subject } from './model.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  /** The values a subject may take: the item types, the buckets, the accounts. */
  export interface Choices {
    types: readonly { value: string; label: string }[];
    buckets: readonly { value: string; label: string }[];
    accounts: readonly { value: string; label: string }[];
  }

  interface Props {
    sentence: Sentence;
    choices: Choices;
    onchange: (next: Sentence) => void;
  }

  const { sentence, choices, onchange }: Props = $props();

  const SUBJECTS: readonly Subject[] = ['type', 'title', 'notes', 'completed', 'archived', 'due', 'assignee', 'bucket', 'parent', 'depth', 'actor', 'hour', 'field'];

  function setSubject(subject: Subject): void {
    const op = OPERATORS[subject][0] ?? 'is';
    const fresh: Sentence = { subject, op };
    if (subject === 'type') fresh.a = choices.types[0]?.value ?? 'TASK';
    if (subject === 'hour') {
      fresh.a = '8';
      fresh.b = '18';
    }
    if (subject === 'bucket') fresh.a = choices.buckets[0]?.value ?? '';
    if (subject === 'depth') fresh.a = '0';
    onchange(fresh);
  }

  function setOp(op: string): void {
    const next: Sentence = { ...sentence, op };
    if (sentence.subject === 'assignee' && (op === 'is' || op === 'is_not') && !next.a) next.a = choices.accounts[0]?.value ?? '';
    if (sentence.subject === 'due' && op === 'within' && !next.a) next.a = '7';
    onchange(next);
  }

  const shape = $derived(takes(sentence.subject, sentence.op));

  const valueOptions = $derived.by(() => {
    if (sentence.subject === 'type') return choices.types;
    if (sentence.subject === 'bucket') return choices.buckets;
    if (sentence.subject === 'assignee' || sentence.subject === 'actor') return choices.accounts;
    return [];
  });

  const read = (event: Event): string => (event.currentTarget as HTMLInputElement | HTMLSelectElement).value;
</script>

<div class="fields">
  <Select label={t('app.flow.composer_subject')} size="sm" value={sentence.subject} options={SUBJECTS.map((subject) => ({ value: subject, label: t(`app.flow.subject_${subject}`) }))} onchange={(event: Event) => setSubject(read(event) as Subject)} />
  <Select label={t('app.flow.composer_op')} size="sm" value={sentence.op} options={OPERATORS[sentence.subject].map((op) => ({ value: op, label: t(`app.flow.op_${op}`) }))} onchange={(event: Event) => setOp(read(event))} />
  {#if shape === 'value'}
    {#if valueOptions.length > 0}
      <Select label={t('app.flow.composer_value')} size="sm" value={sentence.a ?? ''} options={valueOptions} onchange={(event: Event) => onchange({ ...sentence, a: read(event) })} />
    {:else}
      <Input label={t('app.flow.composer_value')} size="sm" value={sentence.a ?? ''} oninput={(event: Event) => onchange({ ...sentence, a: read(event) })} />
    {/if}
  {:else if shape === 'number'}
    <Input label={t('app.flow.composer_value')} size="sm" type="number" min="0" value={sentence.a ?? ''} oninput={(event: Event) => onchange({ ...sentence, a: read(event) })} />
  {:else if shape === 'days'}
    <Input label={t('app.flow.composer_days')} size="sm" type="number" min="0" value={sentence.a ?? ''} oninput={(event: Event) => onchange({ ...sentence, a: read(event) })} />
  {:else if shape === 'range'}
    <Input label={t('app.flow.composer_from')} size="sm" type="number" value={sentence.a ?? ''} oninput={(event: Event) => onchange({ ...sentence, a: read(event) })} />
    <Input label={t('app.flow.composer_to')} size="sm" type="number" value={sentence.b ?? ''} oninput={(event: Event) => onchange({ ...sentence, b: read(event) })} />
  {:else if shape === 'key_value'}
    <Input label={t('app.flow.composer_key')} size="sm" value={sentence.a ?? ''} oninput={(event: Event) => onchange({ ...sentence, a: read(event) })} />
    <Input label={t('app.flow.composer_value')} size="sm" value={sentence.b ?? ''} oninput={(event: Event) => onchange({ ...sentence, b: read(event) })} />
  {/if}
</div>

<style>
  /* The three fields of a sentence side by side where there is room, one under the other where
     there is not: a row in a nested group has less. */
  .fields { display: grid; grid-template-columns: repeat(auto-fit, minmax(12ch, 1fr)); gap: var(--sp-100); }
</style>
