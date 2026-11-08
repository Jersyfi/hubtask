<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A collection's policies as a settings section of its own (page-system §9.6, PS-10, PS-11):
  // whether a child's completion rolls up into its parent, and how what is created here is handed
  // out. One row per setting and one save bar - its own model, a PUT of the whole document, beside
  // *Rules*' structure draft rather than inside it.
  //
  // The values on offer come from the manifest - `completion_policies`, `auto_assign_strategies` -
  // and from nowhere else. **A PUT, and the form says so by being whole**: a key not sent falls
  // back to its default, so every key is written (`containerpolicies.ts`). The rules a policy has
  // to meet are predicted in the server's own codes, under `STRUCTURE`; the server's refusal is
  // what a reader meets when the prediction is wrong.

  import { tick, untrack } from 'svelte';

  import { Button, CapabilityGate, IconButton, SaveBar, Select, SettingRow, SettingsSection, Switch } from '@hubtask/design-system/components';
  import type { Container } from '@hubtask/sync-engine';

  import { accounts } from '../data/accounts.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { holds } from '../data/capability.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { candidateKindOf, candidateProblem, documentOf, draftOf, moved, type Candidate } from '../data/containerpolicies.ts';
  import { groups } from '../data/groups.svelte.ts';
  import { people, type Path } from '../data/people.svelte.ts';
  import { announcer } from '../announce.svelte.ts';
  import { leaveGuard } from '../frame/leave.svelte.ts';
  import { check, focusFirst, isComplete, type Missing } from '../forms/missing.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    container: Container;
    /** The path the collection sits on: who may be named comes from the roles along it. */
    path: Path;
    /** The role this reader holds along the collection, which is what `STRUCTURE` is read from. */
    role: string | undefined;
  }

  const { container, path, role }: Props = $props();

  const structure = $derived(holds(role, 'STRUCTURE'));

  let completion = $state('');
  let strategy = $state('');
  let candidates = $state<Candidate[]>([]);
  let enabled = $state(true);
  let adding = $state('');
  let isSaving = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  /** What the save would be refused for, said once it is pressed. */
  let predicted = $state<string | undefined>(undefined);

  $effect(() => {
    untrack(fill);
    people.open(path);
    return groups.open();
  });

  /** The form as the collection holds it: what opening shows, and what *Discard* goes back to. */
  function fill() {
    const held = draftOf(container.policies);
    completion = held.completion;
    strategy = held.strategy;
    candidates = [...held.candidates];
    enabled = held.enabled;
    adding = '';
    failure = undefined;
    predicted = undefined;
  }

  const draft = $derived({ completion, strategy, candidates, enabled });
  const saved = $derived(draftOf(container.policies));
  const changed = $derived({
    completion: completion !== saved.completion,
    strategy: strategy !== saved.strategy,
    candidates: JSON.stringify(candidates) !== JSON.stringify(saved.candidates),
    enabled: strategy !== '' && enabled !== saved.enabled,
  });
  const changeCount = $derived(Object.values(changed).filter(Boolean).length);
  $effect(() => leaveGuard.hold(() => changeCount));

  /** The installation's values, in its order — never a list compiled in here. */
  const completionOptions = $derived(
    (manifest.value?.completion_policies ?? []).map((value) => ({ value, label: wordFor('completion', value) })),
  );
  const strategyOptions = $derived([
    { value: '', label: t('app.policies.assign_none') },
    ...(manifest.value?.auto_assign_strategies ?? []).map((value) => ({ value, label: wordFor('strategy', value) })),
  ]);

  /** A value's sentence where the catalogue has one, and the value itself where it does not. */
  function wordFor(family: 'completion' | 'strategy', value: string): string {
    const code = `app.policies.${family}_${value.toLowerCase()}`;
    return messages.has(code) ? t(code) : value;
  }

  const wantedKind = $derived(candidateKindOf(strategy));

  /** Who could be named: the accounts along the path, or the workspace's groups, by the strategy. */
  const choices = $derived.by(() => {
    const named = new Set(candidates.map((candidate) => candidate.id));
    if (wantedKind === 'GROUP') {
      return groups.all.filter((group) => !named.has(group.id)).map((group) => ({ value: group.id, label: group.name }));
    }
    return people
      .candidates(path)
      .filter((id) => !named.has(id))
      .map((id) => ({ value: id, label: accounts.nameOf(id) ?? t('app.people.unnamed') }));
  });

  function nameOf(candidate: Candidate): string {
    return candidate.kind === 'GROUP'
      ? (groups.nameOf(candidate.id) ?? t('app.people.unnamed_group'))
      : (accounts.nameOf(candidate.id) ?? t('app.people.unnamed'));
  }

  /** Whether *Add* was pressed with nothing chosen: the choice is marked, never announced before. */
  let addMissing = $state<Missing>({});
  let addRow = $state<HTMLElement | undefined>(undefined);

  function add() {
    addMissing = check([{ kind: 'field', key: 'adding', code: 'app.policies.choose_first', isFilled: adding !== '' }]);
    if (!isComplete(addMissing)) {
      void tick().then(() => focusFirst(addRow));
      return;
    }
    candidates = [...candidates, { kind: wantedKind, id: adding }];
    adding = '';
  }

  // A change of strategy that changes the kind drops the candidates of the other kind: the
  // server would refuse them by name, and a list the reader cannot fix is worse than an empty one.
  $effect(() => {
    const kind = wantedKind;
    if (candidates.some((candidate) => candidate.kind !== kind)) {
      candidates = candidates.filter((candidate) => candidate.kind === kind);
    }
  });

  const problem = $derived(candidateProblem(draft));

  async function save() {
    if (isSaving || changeCount === 0) return;
    // The prediction is said where the save is pressed, never by switching the save off.
    failure = undefined;
    predicted = problem ? t(problem.code, problem.params) : undefined;
    if (predicted) return;
    isSaving = true;
    try {
      await containers.setPolicies(container.id, documentOf(draft), container.version);
      announcer.say(t('app.policies.saved_announced'));
    } catch (error) {
      failure = renderProblem(error as never, messages);
    } finally {
      isSaving = false;
    }
  }
</script>

{#snippet saveBar()}
  <SaveBar
    label={t('app.save_bar.label')}
    changesLabel={changeCount > 0 ? t('app.save_bar.changes', { count: String(changeCount) }) : undefined}
    discardLabel={t('app.save_bar.discard')}
    saveLabel={t('app.workspace.save')}
    {isSaving}
    savingLabel={t('app.workspace.saving')}
    onDiscard={fill}
    onSave={() => void save()}
  />
{/snippet}

<CapabilityGate
  status={structure.status}
  reason={structure.status === 'refused' ? t(structure.code, structure.params) : undefined}
  pendingLabel={t('app.fields.deciding')}
>
  <div data-place-policies>
    <SettingsSection title={t('app.settings.section_policies')} description={t('app.policies.intro')} save={saveBar}>
      {#if predicted ?? failure}<p class="failure" role="alert">{predicted ?? failure?.message}</p>{/if}
      <SettingRow label={t('app.policies.completion')} help={t('app.policies.completion_hint')} fieldWidth="md" isChanged={changed.completion}>
        {#snippet control(nameId)}
          <Select label={t('app.policies.completion')} isLabelHidden aria-labelledby={nameId} bind:value={completion} options={completionOptions} />
        {/snippet}
      </SettingRow>
      <SettingRow label={t('app.policies.assign')} help={strategy ? t(`app.policies.strategy_${strategy.toLowerCase()}_hint`) : t('app.policies.assign_hint')} fieldWidth="md" isChanged={changed.strategy}>
        {#snippet control(nameId)}
          <Select label={t('app.policies.assign')} isLabelHidden aria-labelledby={nameId} bind:value={strategy} options={strategyOptions} />
        {/snippet}
      </SettingRow>
      {#if strategy}
        <SettingRow label={t(wantedKind === 'GROUP' ? 'app.policies.add_group' : 'app.policies.add_person')} fieldWidth="md" isChanged={changed.candidates}>
          {#snippet control()}
            <div class="candidates-control">
              {#if candidates.length > 0}
                <ul class="candidates">
                  {#each candidates as candidate, index (candidate.id)}
                    <li class="candidate">
                      <span class="who">{nameOf(candidate)}</span>
                      <!-- The order matters to the round robin, and only once there is an order. -->
                      {#if strategy === 'ROUND_ROBIN' && candidates.length > 1}
                        <IconButton icon="chevron-up" label={t('app.rank.up')} size="sm"
                          onclick={() => (candidates = moved(candidates, index, -1))}
                          disabledReason={index === 0 ? t('app.rank.already_first') : undefined} />
                        <IconButton icon="chevron-down" label={t('app.rank.down')} size="sm"
                          onclick={() => (candidates = moved(candidates, index, 1))}
                          disabledReason={index === candidates.length - 1 ? t('app.rank.already_last') : undefined} />
                      {/if}
                      <Button size="sm" tone="subtle" onclick={() => (candidates = candidates.filter((c) => c.id !== candidate.id))}>
                        {t('app.policies.remove_candidate')}
                      </Button>
                    </li>
                  {/each}
                </ul>
              {/if}
              <div class="row" bind:this={addRow}>
                <Select
                  label={t(wantedKind === 'GROUP' ? 'app.policies.add_group' : 'app.policies.add_person')}
                  isLabelHidden
                  bind:value={adding}
                  placeholder={t(wantedKind === 'GROUP' ? 'app.policies.choose_group' : 'app.policies.choose_person')}
                  options={choices}
                  error={addMissing.adding && !adding ? t(addMissing.adding) : undefined}
                />
                <Button size="sm" tone="secondary" onclick={add}>{t('app.policies.add')}</Button>
              </div>
            </div>
          {/snippet}
        </SettingRow>
        <SettingRow label={t('app.policies.enabled')} help={t('app.policies.enabled_hint')} fieldWidth="xs" isChanged={changed.enabled}>
          {#snippet control(nameId)}
            <Switch label={t('app.policies.enabled')} isLabelHidden aria-labelledby={nameId} bind:checked={enabled} />
          {/snippet}
        </SettingRow>
      {/if}
    </SettingsSection>
  </div>
</CapabilityGate>

<style>
  .candidates-control { display: flex; flex-direction: column; gap: var(--sp-100); min-inline-size: 0; }
  .candidates { display: flex; flex-direction: column; gap: var(--sp-050); margin: 0; padding: 0; list-style: none; }
  .candidate { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }
  .who { min-inline-size: 0; flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .row { display: flex; flex-wrap: wrap; align-items: end; gap: var(--sp-100); }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); max-inline-size: 64ch; }
</style>
