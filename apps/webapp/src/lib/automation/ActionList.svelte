<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A rule's actions, and the one that contains others.
  //
  // **`BRANCH` nests, so this component recurses.** The contract's `then` and `else` are nested
  // lists rather than jump targets, and the reason is worth keeping in view while editing: "skip
  // the next two" is a rule whose meaning changes when somebody inserts an action above it. A form
  // that flattened the arms would be inventing exactly that fragility.
  //
  // **Both arms are checked as the top level is.** A rule whose `else` performs something its
  // writer may not do is laundering the same rights the day the condition turns false — so this
  // editor treats an arm as a rule body, which is what it is.
  //
  // **The kinds come from the manifest.** `WAIT`, `BRANCH` and `STOP` are the engine's own control
  // structures and are in no catalogue, so they are named here — that is not a compiled-in list of
  // *use cases*, it is the three things the contract says are not use cases at all.

  import { Button, Input, Select, Stack, Textarea } from '@hubtask/design-system/components';

  // The self-import is how Svelte 5 recurses; `<svelte:self>` is deprecated.
  import ActionList from './ActionList.svelte';

  import type { DraftAction } from '../data/rules.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    /** The list being edited. */
    actions: readonly DraftAction[];
    /** The action kinds this installation serves, from `/meta/capabilities`. */
    kinds: readonly string[];
    /** The list as it should be afterwards. A callback rather than a binding, because the same
     *  component edits a branch's arm one level down and one shape for both is what keeps the
     *  recursion honest. */
    onchange: (next: DraftAction[]) => void;
    /** How deep this list sits. Only for the shading a nested arm gets. */
    depth?: number;
  }

  const { actions, kinds, onchange, depth = 0 }: Props = $props();

  /** Every edit goes through here: take the list, change one entry, hand it back. */
  const replace = (at: number, action: DraftAction) =>
    onchange(actions.map((each, index) => (index === at ? action : each)));

  /** The three the contract calls the engine's own, which no catalogue lists. */
  const CONTROL = ['WAIT', 'BRANCH', 'STOP'];

  const options = $derived([
    ...kinds.map((kind) => ({ value: kind, label: kind })),
    ...CONTROL.map((kind) => ({ value: kind, label: t(`app.rules.control_${kind.toLowerCase()}`) })),
  ]);

  function add(): void {
    onchange([...actions, { kind: '' }]);
  }

  function removeAt(index: number): void {
    onchange(actions.filter((_, at) => at !== index));
  }

  function setKind(index: number, kind: string): void {
    // The parameters belong to the kind, so changing the kind clears them: a `duration` left over
    // from a `WAIT` would be a key the next action's use case does not declare, refused by the
    // server for a reason the reader cannot see.
    replace(index, kind === 'BRANCH' ? { kind, params: {}, then: [], else: [] } : { kind });
  }
</script>

<Stack gap="150">
  {#each actions as action, index (index)}
    <div class="action" data-depth={depth}>
      <Stack gap="100">
        <Select
          label={t('app.rules.action_kind')}
          value={action.kind}
          placeholder={t('app.rules.choose_action')}
          options={options}
          onchange={(event: Event) => setKind(index, (event.currentTarget as HTMLSelectElement).value)}
        />

        {#if action.kind === 'WAIT'}
          <Input
            label={t('app.rules.wait_duration')}
            hint={t('app.rules.wait_duration_hint')}
            value={String(action.params?.duration ?? '')}
            oninput={(event: Event) =>
              replace(index, {
                ...action,
                params: { duration: (event.currentTarget as HTMLInputElement).value },
              })}
          />
        {:else if action.kind === 'BRANCH'}
          <Input
            label={t('app.rules.branch_condition')}
            hint={t('app.rules.branch_condition_hint')}
            value={String(action.params?.condition ?? '')}
            oninput={(event: Event) =>
              replace(index, {
                ...action,
                params: { condition: (event.currentTarget as HTMLInputElement).value },
              })}
          />

          <!-- The recursion, once per arm. Both arms are checked as the top level is, so both are
               edited as one: a rule whose `else` performs something its writer may not do is
               laundering the same rights the day the condition turns false. -->
          <span class="label">{t('app.rules.branch_then')}</span>
          <ActionList
            actions={action.then ?? []}
            {kinds}
            depth={depth + 1}
            onchange={(next) => replace(index, { ...action, then: next })}
          />

          <span class="label">{t('app.rules.branch_else')}</span>
          <ActionList
            actions={action.else ?? []}
            {kinds}
            depth={depth + 1}
            onchange={(next) => replace(index, { ...action, else: next })}
          />
        {:else if action.kind === 'STOP'}
          <p class="quiet small">{t('app.rules.stop_hint')}</p>
        {:else if action.kind}
          <Textarea
            label={t('app.rules.action_params')}
            hint={t('app.rules.action_params_hint')}
            value={action.paramsText ?? ''}
            rows={2}
            spellcheck={false}
            oninput={(event: Event) =>
              replace(index, {
                ...action,
                paramsText: (event.currentTarget as HTMLTextAreaElement).value,
              })}
          />
        {/if}

        <div>
          <Button size="sm" tone="subtle" onclick={() => removeAt(index)}>
            {t('app.rules.remove_action')}
          </Button>
        </div>
      </Stack>
    </div>
  {/each}

  <div>
    <Button size="sm" tone="secondary" onclick={add}>{t('app.rules.add_action')}</Button>
  </div>
</Stack>

<style>
  .action {
    padding: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    background: var(--bg-surface);
  }

  /* A nested arm sits inside its branch rather than beside it, because that is what it is. */
  .action[data-depth='1'] { background: var(--bg-surface-sunken); }

  .label { color: var(--text-primary); font-size: var(--fs-075); font-weight: var(--fw-medium); }

  .quiet { margin: 0; color: var(--text-secondary); }

  .small { font-size: var(--fs-075); }
</style>
