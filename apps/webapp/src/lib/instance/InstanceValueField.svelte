<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One key of the installation's defaults, edited with the control of its kind (UC-INS-09 checks
  // 1 to 3): a count is a number field within the product's bounds, a flag and a choice are
  // choices in words, a list of choices is a set of checkboxes, a link an address, a language and
  // a zone the lists the workspace's own screen offers.
  //
  // **Every control can say "not decided"**, because that is a value of the level: the key is the
  // workspace's to decide. A switch cannot say it, which is why a flag is a choice here.
  //
  // **The lock beside it says what a workspace may still do**, in the words of the key's kind - it
  // may make a rule stricter, raise a limit, use its own link. A key without a value has nothing to
  // lock, and a language, a zone and a week start are defaults that are never locked.

  import { Checkbox, Input, Select } from '@hubtask/design-system/components';
  import type { InstanceSetting } from '@hubtask/sync-engine';

  import { t } from '../i18n/i18n.svelte.ts';
  import { choiceCode, countOf, lockCode, nameCode, type Area } from './values.ts';

  interface Props {
    area: Area;
    key: string;
    /** What the server answered, with the key's declaration. */
    declared: InstanceSetting;
    /** What the draft holds now; undefined is undecided. */
    current: InstanceSetting | undefined;
    locales: readonly string[];
    zones: readonly string[];
    error?: string;
    onchange: (next: InstanceSetting | undefined) => void;
  }

  const { area, key, declared, current, locales, zones, error, onchange }: Props = $props();

  const label = $derived(t(nameCode(area, key)));
  const decided = $derived(current?.set === true);
  const locked = $derived(current?.locked === true);
  const undecided = $derived(t('app.instance.value_undecided'));

  /** Puts a value, keeping the lock it had; undefined clears the key and its lock with it. */
  function put(value: unknown): void {
    if (value === undefined || value === '') {
      onchange(undefined);
      return;
    }
    onchange({ ...declared, set: true, value, locked });
  }

  const choices = $derived(
    (declared.options ?? []).map((option) => ({
      value: String(option),
      label: t(choiceCode(area, key, String(option))),
    })),
  );

  const chosen = $derived(decided && Array.isArray(current?.value) ? (current?.value as string[]) : []);

  function toggle(option: string, on: boolean): void {
    const next = (declared.options ?? []).map(String).filter((each) => (each === option ? on : chosen.includes(each)));
    put(next.length === 0 ? undefined : next);
  }

  const lockOpen = $derived(lockCode(declared, false));
  const lockClosed = $derived(lockCode(declared, true));
</script>

<div class="value" data-value={`${area}.${key}`}>
  {#if declared.kind === 'COUNT'}
    <Input
      {label}
      type="number"
      inputmode="numeric"
      min={declared.minimum}
      max={declared.maximum}
      value={decided ? String(current?.value ?? '') : ''}
      placeholder={undecided}
      oninput={(event) => put(countOf((event.currentTarget as HTMLInputElement).value))}
      {error}
    />
  {:else if declared.kind === 'FLAG'}
    <Select
      {label}
      value={decided ? String(current?.value) : ''}
      options={[
        { value: '', label: undecided },
        { value: 'true', label: t('app.instance.flag_on') },
        { value: 'false', label: t('app.instance.flag_off') },
      ]}
      onchange={(event) => {
        const raw = (event.currentTarget as HTMLSelectElement).value;
        put(raw === '' ? undefined : raw === 'true');
      }}
      {error}
    />
  {:else if declared.kind === 'CHOICE'}
    <Select
      {label}
      value={decided ? String(current?.value) : ''}
      options={[{ value: '', label: undecided }, ...choices]}
      onchange={(event) => put((event.currentTarget as HTMLSelectElement).value)}
      {error}
    />
  {:else if declared.kind === 'CHOICES'}
    <fieldset class="choices">
      <legend>{label}</legend>
      <p class="hint">{decided ? '' : undecided}</p>
      {#each choices as choice (choice.value)}
        <Checkbox
          label={choice.label}
          checked={chosen.includes(choice.value)}
          onchange={(event) => toggle(choice.value, (event.currentTarget as HTMLInputElement).checked)}
        />
      {/each}
      {#if error}<p class="refusal" role="alert">{error}</p>{/if}
    </fieldset>
  {:else if declared.kind === 'LOCALE' && locales.length > 0}
    <Select
      {label}
      value={decided ? String(current?.value) : ''}
      options={[{ value: '', label: undecided }, ...locales.map((each) => ({ value: each, label: each }))]}
      onchange={(event) => put((event.currentTarget as HTMLSelectElement).value)}
      {error}
    />
  {:else if declared.kind === 'TIME_ZONE' && zones.length > 0}
    <Select
      {label}
      value={decided ? String(current?.value) : ''}
      options={[{ value: '', label: undecided }, ...zones.map((each) => ({ value: each, label: each }))]}
      onchange={(event) => put((event.currentTarget as HTMLSelectElement).value)}
      {error}
    />
  {:else}
    <!-- A link, and a language or a zone where this installation lists none to choose from. -->
    <Input
      {label}
      type={declared.kind === 'LINK' ? 'url' : 'text'}
      value={decided ? String(current?.value ?? '') : ''}
      placeholder={undecided}
      oninput={(event) => put((event.currentTarget as HTMLInputElement).value.trim())}
      autocomplete="off"
      spellcheck={false}
      {error}
    />
  {/if}

  {#if lockOpen && lockClosed}
    <Select
      label={t('app.instance.workspaces_may')}
      value={locked ? 'locked' : 'open'}
      options={[
        { value: 'open', label: t(lockOpen) },
        { value: 'locked', label: t(lockClosed) },
      ]}
      disabledReason={decided ? undefined : t('app.instance.lock_needs_value')}
      onchange={(event) => {
        if (!current) return;
        onchange({ ...current, locked: (event.currentTarget as HTMLSelectElement).value === 'locked' });
      }}
    />
  {/if}
</div>

<style>
  .value {
    display: grid;
    gap: var(--sp-100);
    /* The value and its lock side by side where there is room, one above the other on a phone. */
    grid-template-columns: repeat(auto-fit, minmax(24ch, 1fr));
    align-items: start;
    padding-block-end: var(--sp-150);
    border-block-end: var(--bw-hairline) solid var(--border-subtle);
  }

  .choices { margin: 0; padding: 0; border: 0; display: grid; gap: var(--sp-050); }

  .choices legend {
    padding: 0;
    color: var(--text-primary);
    font-size: var(--fs-075);
    font-weight: var(--fw-medium);
  }

  .hint { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }

  .refusal { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
</style>
