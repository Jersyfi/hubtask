<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Place a hold* as a Dialog M (page-system §9.3): what it covers - one of four scopes
  // (UC-LIF-06 check 8) -, the person, hub or entry it names, picked rather than typed (P-12), and
  // why, which is kept and is what an auditor reads.
  //
  // **`:retain` is not a hold**, and the dialog says which is which: one takes a single entry out
  // of a running period, the other overrides every rule and reaches everything under it.
  import { tick, untrack } from 'svelte';

  import { Button, Dialog, SearchField, Select, Stack, Textarea } from '@hubtask/design-system/components';

  import { announcer } from '../announce.svelte.ts';
  import { accounts } from '../data/accounts.svelte.ts';
  import { policies } from '../data/policies.svelte.ts';
  import { search } from '../data/search.svelte.ts';
  import { check, focusFirst, isComplete, type Missing } from '../forms/missing.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import DiscardDialog from '../page/DiscardDialog.svelte';
  import { DiscardGuard } from '../page/discard.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    isOpen: boolean;
    /** The workspace's people, by account. */
    people: readonly string[];
    /** Its hubs with their collections. */
    places: readonly { value: string; label: string }[];
  }

  let { isOpen = $bindable(false), people, places }: Props = $props();

  /** What a hold can cover. `TENANT` names nothing because it covers everything. */
  const SCOPES = ['TENANT', 'CONTAINER', 'ITEM', 'ACCOUNT'];
  /** The entries a search offers: a picker list never longer than a Dialog M holds (§5.1). */
  const HITS = 8;

  let scope = $state('TENANT');
  let target = $state('');
  /** The entry a hold on one entry names, found by its title rather than typed (P-08). */
  let term = $state('');
  let entry = $state<{ id: string; title: string } | undefined>(undefined);
  let reason = $state('');
  let missing = $state<Missing>({});
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isWorking = $state(false);
  let form = $state<HTMLFormElement | undefined>(undefined);

  $effect(() => {
    const question = scope === 'ITEM' ? term.trim() : '';
    if (question === '') {
      untrack(() => search.reset());
      return;
    }
    const timer = setTimeout(() => void search.run({ q: question }), 250);
    return () => clearTimeout(timer);
  });

  function reset(): void {
    scope = 'TENANT';
    target = '';
    term = '';
    entry = undefined;
    reason = '';
    missing = {};
    failure = undefined;
  }

  function close(): void {
    isOpen = false;
  }

  const guard = new DiscardGuard(() => scope !== 'TENANT' || reason.trim() !== '', close);

  async function place(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    missing = check([{ kind: 'field', key: 'reason', code: 'app.forms.required', isFilled: reason.trim() !== '' }]);
    if (!isComplete(missing)) {
      void tick().then(() => focusFirst(form));
      return;
    }
    failure = undefined;
    isWorking = true;
    try {
      await policies.place({ kind: scope, id: target.trim() || null }, reason.trim());
      announcer.say(t('app.retention.placed_announced'));
      close();
    } catch (cause) {
      failure = renderProblem(cause as never, messages);
    } finally {
      isWorking = false;
    }
  }
</script>

<Dialog
  size="m"
  title={t('app.retention.place_title')}
  bind:isOpen
  dismissLabel={t('app.workspace.cancel')}
  onDismiss={guard.dismiss}
  onClose={reset}
  primary={{ label: t('app.retention.place'), isBusy: isWorking, busyLabel: t('app.retention.placing'), onclick: () => form?.requestSubmit() }}
>
  <form bind:this={form} onsubmit={place}>
    <Stack gap="150">
      {#if failure}<FailureNote problem={failure} />{/if}
      <Select
        label={t('app.retention.hold_scope')}
        bind:value={scope}
        onchange={() => {
          target = '';
          entry = undefined;
        }}
        options={SCOPES.map((kind) => ({ value: kind, label: t(`app.retention.hold_scope_${kind.toLowerCase()}`) }))}
      />
      {#if scope === 'ACCOUNT'}
        <Select
          label={t('app.retention.hold_person')}
          hint={t('app.retention.hold_person_note')}
          bind:value={target}
          placeholder={t('app.retention.choose_person')}
          options={people.map((id) => ({ value: id, label: accounts.nameOf(id) ?? id }))}
        />
      {:else if scope === 'CONTAINER'}
        <Select
          label={t('app.retention.hold_container')}
          bind:value={target}
          placeholder={t('app.retention.choose_container')}
          options={places}
        />
      {:else if scope === 'ITEM'}
        <Stack gap="050">
          <SearchField
            label={t('app.retention.hold_entry')}
            clearLabel={t('app.search.clear')}
            bind:value={term}
            onclear={() => {
              target = '';
              entry = undefined;
            }}
          />
          <span class="quiet">{t('app.retention.hold_entry_hint')}</span>
          {#if entry}
            <span class="name">{t('app.retention.hold_entry_chosen', { title: entry.title })}</span>
          {/if}
          {#if term.trim() !== '' && search.status === 'done'}
            {#if search.hits.length === 0}
              <span class="quiet">{t('app.retention.hold_entry_none')}</span>
            {:else}
              <ul class="hits">
                {#each search.hits.slice(0, HITS) as hit (hit.id)}
                  <li>
                    <button
                      class="hit"
                      type="button"
                      aria-pressed={entry?.id === hit.id}
                      onclick={() => {
                        entry = { id: hit.id, title: hit.title };
                        target = hit.id;
                      }}>{hit.title}</button
                    >
                  </li>
                {/each}
              </ul>
            {/if}
          {/if}
        </Stack>
      {/if}
      <Textarea
        label={t('app.retention.hold_reason_field')}
        hint={t('app.retention.hold_reason_hint')}
        bind:value={reason}
        rows={2}
        isRequired
        error={missing.reason && !reason.trim() ? t(missing.reason) : undefined}
      />
      <p class="quiet">{t('app.retention.hold_not_retain')}</p>
    </Stack>
  </form>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => { if (guard.dismiss()) close(); }}>{t('app.workspace.cancel')}</Button>
  {/snippet}
</Dialog>
<DiscardDialog {guard} />

<style>
  form { margin: 0; }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .name { color: var(--text-primary); font-weight: var(--fw-medium); }

  /* An entry found for a hold: the same list of titles the probe's subject search draws. */
  .hits { margin: 0; padding: 0; list-style: none; display: flex; flex-direction: column; gap: var(--sp-025); }
  .hit { inline-size: 100%; text-align: start; padding: var(--sp-050) var(--sp-100); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-sm); background: var(--bg-surface); color: var(--text-primary); font-size: var(--fs-075); }
  .hit:hover { background: var(--bg-surface-hover); }
  .hit[aria-pressed='true'] { border-color: var(--accent-primary); background: var(--accent-primary-subtle); }
  .hit:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }
</style>
