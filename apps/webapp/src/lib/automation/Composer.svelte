<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A condition composed as a sentence and stored as CEL (F8-04, decision 3).
  //
  // The composer offers a bounded set of subjects with the operators each takes, compiles to the
  // expression the server stores, and shows that expression under the sentence. *Edit as an
  // expression* switches to the raw text the server already accepts. An expression the composer
  // did not write is shown as an expression, with the offer to replace it by a sentence - never
  // silently rewritten, because it may say something the sentences cannot.

  import { Checkbox, Input, Select, Stack, Textarea } from '@hubtask/design-system/components';

  import { compileSentence, defaultSentence, OPERATORS, readSentence, takes, type Sentence, type Subject } from './model.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    /** The expression as it stands. */
    expr: string;
    /** The server's refusal at this expression, where there is one. */
    error?: string;
    /** The values a subject may take: the item types, the buckets, the accounts. */
    choices: {
      types: readonly { value: string; label: string }[];
      buckets: readonly { value: string; label: string }[];
      accounts: readonly { value: string; label: string }[];
    };
    onchange: (expr: string) => void;
  }

  const { expr, error, choices, onchange }: Props = $props();

  /** Expert mode is the reader's choice, kept while the same condition is edited. */
  let expert = $state(false);

  const sentence = $derived(readSentence(expr));
  const foreign = $derived(expr.trim() !== '' && sentence === undefined);
  const editingRaw = $derived(expert || foreign);

  const SUBJECTS: readonly Subject[] = ['type', 'completed', 'due', 'assignee', 'bucket', 'parent', 'actor', 'hour', 'field'];

  function emit(next: Sentence): void {
    onchange(compileSentence(next));
  }

  function setSubject(subject: Subject): void {
    const op = OPERATORS[subject][0] ?? 'is';
    const fresh: Sentence = { subject, op };
    if (subject === 'type') fresh.a = choices.types[0]?.value ?? 'TASK';
    if (subject === 'hour') {
      fresh.a = '8';
      fresh.b = '18';
    }
    if (subject === 'bucket') fresh.a = choices.buckets[0]?.value ?? '';
    emit(fresh);
  }

  function setOp(op: string): void {
    const current = sentence ?? defaultSentence();
    const next: Sentence = { ...current, op };
    if (current.subject === 'assignee' && op === 'is' && !next.a) next.a = choices.accounts[0]?.value ?? '';
    emit(next);
  }

  const valueOptions = $derived.by(() => {
    if (!sentence) return [];
    if (sentence.subject === 'type') return choices.types;
    if (sentence.subject === 'bucket') return choices.buckets;
    if (sentence.subject === 'assignee' || sentence.subject === 'actor') return choices.accounts;
    return [];
  });
</script>

<Stack gap="150">
  {#if editingRaw}
    <Textarea
      label={t('app.rules.condition')}
      hint={t('app.flow.composer_expert_hint')}
      {error}
      rows={3}
      value={expr}
      spellcheck={false}
      oninput={(event: Event) => onchange((event.currentTarget as HTMLTextAreaElement).value)}
    />
    {#if foreign}
      <p class="foreign">{t('app.flow.composer_foreign')}</p>
      <button class="link" type="button" onclick={() => emit(defaultSentence())}>{t('app.flow.composer_replace')}</button>
    {:else}
      <Checkbox label={t('app.flow.composer_expert')} checked={expert} onchange={() => (expert = !expert)} />
    {/if}
  {:else}
    {@const current = sentence ?? defaultSentence()}
    {@const shape = takes(current.subject, current.op)}
    <Select
      label={t('app.flow.composer_subject')}
      value={current.subject}
      options={SUBJECTS.map((subject) => ({ value: subject, label: t(`app.flow.subject_${subject}`) }))}
      onchange={(event: Event) => setSubject((event.currentTarget as HTMLSelectElement).value as Subject)}
    />
    <Select
      label={t('app.flow.composer_op')}
      value={current.op}
      options={OPERATORS[current.subject].map((op) => ({ value: op, label: t(`app.flow.op_${op}`) }))}
      onchange={(event: Event) => setOp((event.currentTarget as HTMLSelectElement).value)}
    />
    {#if shape === 'value'}
      {#if valueOptions.length > 0}
        <Select
          label={t('app.flow.composer_value')}
          value={current.a ?? ''}
          options={valueOptions}
          onchange={(event: Event) => emit({ ...current, a: (event.currentTarget as HTMLSelectElement).value })}
        />
      {:else}
        <Input
          label={t('app.flow.composer_value')}
          value={current.a ?? ''}
          oninput={(event: Event) => emit({ ...current, a: (event.currentTarget as HTMLInputElement).value })}
        />
      {/if}
    {:else if shape === 'range'}
      <div class="two">
        <Input label={t('app.flow.composer_from')} type="number" value={current.a ?? ''} oninput={(event: Event) => emit({ ...current, a: (event.currentTarget as HTMLInputElement).value })} />
        <Input label={t('app.flow.composer_to')} type="number" value={current.b ?? ''} oninput={(event: Event) => emit({ ...current, b: (event.currentTarget as HTMLInputElement).value })} />
      </div>
    {:else if shape === 'key_value'}
      <Input label={t('app.flow.composer_key')} value={current.a ?? ''} oninput={(event: Event) => emit({ ...current, a: (event.currentTarget as HTMLInputElement).value })} />
      <Input label={t('app.flow.composer_value')} value={current.b ?? ''} oninput={(event: Event) => emit({ ...current, b: (event.currentTarget as HTMLInputElement).value })} />
    {/if}
    <div>
      <span class="label">{t('app.flow.composer_compiled')}</span>
      <code class="compiled" class:refused={Boolean(error)}>{expr || compileSentence(current)}</code>
      {#if error}<span class="error">{error}</span>{/if}
    </div>
    <Checkbox label={t('app.flow.composer_expert')} checked={expert} onchange={() => (expert = !expert)} />
  {/if}
</Stack>

<style>
  .two { display: grid; grid-template-columns: 1fr 1fr; gap: var(--sp-100); }

  .label { display: block; margin-block-end: var(--sp-050); font-size: var(--fs-075); font-weight: var(--fw-medium); }

  .compiled {
    display: block;
    padding: var(--sp-100) var(--sp-150);
    border-radius: var(--r-sm);
    background: var(--bg-surface-sunken);
    color: var(--text-secondary);
    font-family: var(--font-mono);
    font-size: var(--fs-075);
    overflow-wrap: anywhere;
  }

  .compiled.refused { outline: var(--bw-hairline) solid var(--danger-500); }

  .error { display: block; margin-block-start: var(--sp-050); font-size: var(--fs-075); color: var(--text-danger); }

  .foreign { margin: 0; font-size: var(--fs-075); color: var(--text-secondary); }

  .link { align-self: flex-start; padding: 0; border: 0; background: none; color: var(--text-brand); font-size: var(--fs-075); font-weight: var(--fw-medium); cursor: pointer; }

  .link:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); border-radius: var(--r-xs); }
</style>
