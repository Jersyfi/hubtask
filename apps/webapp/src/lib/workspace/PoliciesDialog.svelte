<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A collection's policies: how it works, as opposed to what it is called (issue 773).
  //
  // Two keys today, both the contract's: whether a child's completion rolls up into its parent,
  // and how what is created here is handed out. The values on offer come from the manifest -
  // `completion_policies`, `auto_assign_strategies` - and from nowhere else, so an installation
  // that serves one more offers one more here without a client change.
  //
  // **A PUT, and the form says so by being whole.** The document is replaced, and a key that is
  // not sent falls back to its default; the form therefore holds every key and writes every one
  // it has a value for (`containerpolicies.ts`). The three rules a policy has to meet are
  // predicted in the server's own codes, and under `STRUCTURE` for the reason the custom fields
  // are: the gate is a prediction, and the server's refusal is what a reader meets when it is
  // wrong.

  import {
    Button,
    CapabilityGate,
    Dialog,
    IconButton,
    Select,
    Stack,
    Switch,
  } from '@hubtask/design-system/components';
  import type { Container } from '@hubtask/sync-engine';

  import { accounts } from '../data/accounts.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { holds } from '../data/capability.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import {
    candidateKindOf,
    candidateProblem,
    documentOf,
    draftOf,
    moved,
    type Candidate,
  } from '../data/containerpolicies.ts';
  import { groups } from '../data/groups.svelte.ts';
  import { people, type Path } from '../data/people.svelte.ts';
  import { announcer } from '../announce.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    isOpen: boolean;
    container: Container;
    /** The path the collection sits on: who may be named comes from the roles along it. */
    path: Path;
    /** The role this reader holds along the collection, which is what `STRUCTURE` is read from. */
    role: string | undefined;
  }

  let { isOpen = $bindable(), container, path, role }: Props = $props();

  const structure = $derived(holds(role, 'STRUCTURE'));

  let completion = $state('');
  let strategy = $state('');
  let candidates = $state<Candidate[]>([]);
  let enabled = $state(true);
  let adding = $state('');
  let isSaving = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  // Opening fills the form from the collection. Closing leaves it alone, for `BucketDialog`'s
  // reason: the fields are re-read on the next open.
  $effect(() => {
    if (!isOpen) return;
    const draft = draftOf(container.policies);
    completion = draft.completion;
    strategy = draft.strategy;
    candidates = [...draft.candidates];
    enabled = draft.enabled;
    adding = '';
    failure = undefined;
    people.open(path);
    return groups.open();
  });

  const draft = $derived({ completion, strategy, candidates, enabled });

  /** The installation's values, in its order — never a list compiled in here. */
  const completionOptions = $derived([
    { value: '', label: t('app.policies.completion_default') },
    ...(manifest.value?.completion_policies ?? []).map((value) => ({ value, label: wordFor('completion', value) })),
  ]);
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

  function add() {
    if (!adding) return;
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
    if (isSaving || problem) return;
    isSaving = true;
    failure = undefined;
    try {
      await containers.setPolicies(container.id, documentOf(draft), container.version);
      announcer.say(t('app.policies.saved_announced'));
      isOpen = false;
    } catch (error) {
      failure = renderProblem(error as never, messages);
    } finally {
      isSaving = false;
    }
  }
</script>

<Dialog bind:isOpen title={t('app.policies.title')} dismissLabel={t('app.workspace.cancel')}>
  <CapabilityGate
    status={structure.status}
    reason={structure.status === 'refused' ? t(structure.code, structure.params) : undefined}
    pendingLabel={t('app.fields.deciding')}
  >
    <Stack gap="200">
      <p class="hint">{t('app.policies.intro')}</p>

      <Select
        label={t('app.policies.completion')}
        hint={t('app.policies.completion_hint')}
        bind:value={completion}
        options={completionOptions}
      />

      <Stack gap="100">
        <Select
          label={t('app.policies.assign')}
          hint={t('app.policies.assign_hint')}
          bind:value={strategy}
          options={strategyOptions}
        />
        {#if strategy}
          <p class="hint">{t(`app.policies.strategy_${strategy.toLowerCase()}_hint`)}</p>
          {#if candidates.length > 0}
            <ul class="candidates">
              {#each candidates as candidate, index (candidate.id)}
                <li class="candidate">
                  <span class="who">{nameOf(candidate)}</span>
                  {#if strategy === 'ROUND_ROBIN'}
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
          <div class="row">
            <Select
              label={t(wantedKind === 'GROUP' ? 'app.policies.add_group' : 'app.policies.add_person')}
              bind:value={adding}
              placeholder={t(wantedKind === 'GROUP' ? 'app.policies.choose_group' : 'app.policies.choose_person')}
              options={choices}
            />
            <Button size="sm" tone="secondary" onclick={add} disabledReason={adding ? undefined : t('app.policies.choose_first')}>
              {t('app.policies.add')}
            </Button>
          </div>
          <Switch label={t('app.policies.enabled')} hint={t('app.policies.enabled_hint')} bind:checked={enabled} />
        {/if}
      </Stack>

      {#if problem}<p class="failure" role="alert">{t(problem.code, problem.params)}</p>{/if}
      {#if failure}<p class="failure" role="alert">{failure.message}</p>{/if}
    </Stack>
  </CapabilityGate>

  {#snippet actions()}
    <Button tone="secondary" onclick={() => (isOpen = false)}>{t('app.workspace.cancel')}</Button>
    <Button
      isBusy={isSaving}
      busyLabel={t('app.workspace.saving')}
      disabledReason={problem ? t(problem.code, problem.params) : structure.status === 'permitted' ? undefined : t('app.fields.deciding')}
      onclick={save}
    >
      {t('app.workspace.save')}
    </Button>
  {/snippet}
</Dialog>

<style>
  .hint { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); max-width: 64ch; }

  .candidates { display: flex; flex-direction: column; gap: var(--sp-050); margin: 0; padding: 0; list-style: none; }

  .candidate { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }

  .who { min-width: 0; flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

  .row { display: flex; flex-wrap: wrap; align-items: end; gap: var(--sp-100); }

  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); max-width: 64ch; }
</style>
