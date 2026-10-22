<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The workspace's position on people keeping their own content out of AI (UC-PRV-05 checks 9,
  // 11; data-protection.md §4.1), on its AI screen - the one place it is set (P-06).
  //
  // **Each person may keep their content out** is the default. **AI is part of everybody's work**
  // needs a legal basis the workspace names; pressed without one, the basis is marked, as the
  // server would refuse it (UC-WRK-32 check 3). Where the installation or a plan holds the
  // workspace at the default, the position in force is shown with the chip that says who decided
  // and whom to ask, and no control is offered (P-05, P-07, UC-WRK-32 check 5).
  //
  // **A change shows in the save bar** - "2 changes · Discard · Save" - rather than a save that is
  // off until something moved (check 3), and the save confirms as a passing message (check 6).
  //
  // **Alone, nothing is asked** (P-10, UC-PRV-05 check 8): with one person there is nobody whose
  // content could be kept out, so the choice is not drawn. A stored *AI for everybody* stays
  // drawn, because it is still in force and only here can it be taken back.
  //
  // **How many people it touches is said before it is done** (P-04): a number, never who - the
  // consent register on *Data subject requests* is where names are.

  import { tick, untrack } from 'svelte';

  import { Input, Radio, SaveBar, Select, Stack } from '@hubtask/design-system/components';
  import type { AiOptOutBasis, AiPersonOptOut, AiSource } from '@hubtask/sync-engine';

  import { aiProvider } from '../data/aiprovider.svelte.ts';
  import { workspace } from '../data/workspace.svelte.ts';
  import { enterSubmits } from '../forms/enter.ts';
  import { check, focusFirst, isComplete, type Missing } from '../forms/missing.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import ValueSource from './ValueSource.svelte';
  import WriteOnly from './WriteOnly.svelte';

  interface Props {
    source: AiSource;
  }

  const { source }: Props = $props();

  const BASES: readonly AiOptOutBasis[] = ['EMPLOYMENT_CONTRACT', 'WORKS_AGREEMENT', 'LEGAL_OBLIGATION'];

  let position = $state<string>('OFFERED');
  let basis = $state<string>('');
  let contact = $state('');
  let isSaving = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let missing = $state<Missing>({});
  let form = $state<HTMLFormElement | undefined>(undefined);

  /** What is stored, as the form holds it. */
  const stored = $derived({
    position: source.person_opt_out,
    basis: source.person_opt_out_basis ?? '',
    contact: source.person_opt_out_contact ?? '',
  });

  /** The fields that differ from what is stored; basis and contact only count where they apply. */
  const changed = $derived(
    [
      position !== stored.position ? 'position' : undefined,
      position === 'NOT_OFFERED' && basis !== stored.basis ? 'basis' : undefined,
      position === 'NOT_OFFERED' && contact.trim() !== stored.contact ? 'contact' : undefined,
    ].filter((each): each is string => each !== undefined),
  );

  // Seeded from what the server holds, and again whenever it answers - but not over a change the
  // reader is still holding: a re-read in the middle of an edit would throw the edit away.
  $effect(() => {
    const next = stored;
    untrack(() => {
      if (changed.length > 0) return;
      seed(next);
    });
  });

  function seed(values: { position: string; basis: string; contact: string }) {
    position = values.position;
    basis = values.basis;
    contact = values.contact;
  }

  function discard() {
    failure = undefined;
    missing = {};
    seed(stored);
  }

  const isAsked = $derived(
    workspace.workspace?.several_people === true || source.person_opt_out === 'NOT_OFFERED',
  );
  const isLocked = $derived(Boolean(source.person_opt_out_lock));
  const lockedBy = $derived(
    source.person_opt_out_lock
      ? source.person_opt_out_lock === 'PLAN'
        ? t('app.ai.set_by_plan')
        : t('app.ai.set_by_installation')
      : undefined,
  );
  const isMovingToEverybody = $derived(position === 'NOT_OFFERED' && source.person_opt_out !== 'NOT_OFFERED');

  async function save(event: SubmitEvent) {
    event.preventDefault();
    if (changed.length === 0) return;
    missing = check([
      { kind: 'field', key: 'basis', code: 'app.ai.opt_out_basis_needed', isFilled: position !== 'NOT_OFFERED' || basis !== '' },
    ]);
    if (!isComplete(missing)) {
      void tick().then(() => focusFirst(form));
      return;
    }
    isSaving = true;
    failure = undefined;
    try {
      await aiProvider.setPersonOptOut(
        position === 'NOT_OFFERED'
          ? { person_opt_out: 'NOT_OFFERED', basis: basis as AiOptOutBasis, contact: contact.trim() }
          : { person_opt_out: 'OFFERED' as AiPersonOptOut },
      );
      toast.show({ text: t('app.ai.opt_out_saved'), tone: 'success' });
    } catch (error) {
      failure = renderProblem(error as never, messages);
    } finally {
      isSaving = false;
    }
  }
</script>

{#if isAsked}
<section class="panel" aria-labelledby="person-opt-out-title" data-person-opt-out>
  <Stack gap="200">
    <h2 class="section" id="person-opt-out-title">{t('app.ai.opt_out_title')}</h2>
    <p class="quiet">{t('app.ai.opt_out_intro')}</p>

    {#if isLocked}
      <!-- The position in force, as a value with where it comes from - not a form with nothing in it. -->
      <p class="value">
        <strong>{t(source.person_opt_out === 'NOT_OFFERED' ? 'app.ai.opt_out_not_offered' : 'app.ai.opt_out_offered')}</strong>
        <ValueSource
          source={source.person_opt_out_origin}
          lock={source.person_opt_out_lock}
          value={t(source.person_opt_out === 'NOT_OFFERED' ? 'app.ai.opt_out_not_offered' : 'app.ai.opt_out_offered')}
        />
      </p>
      <p class="quiet">{t('app.ai.opt_out_locked', { by: lockedBy ?? '' })}</p>
    {:else}
      <form bind:this={form} onsubmit={save} use:enterSubmits>
        <Stack gap="200">
          <Radio
            label={t('app.ai.opt_out_position')}
            bind:value={position}
            options={[
              { value: 'OFFERED', label: t('app.ai.opt_out_offered') },
              { value: 'NOT_OFFERED', label: t('app.ai.opt_out_not_offered') },
            ]}
          />
          {#if position === 'NOT_OFFERED'}
            <div class="fields">
              <Stack gap="200">
                <Select
                  label={t('app.ai.opt_out_basis')}
                  hint={t('app.ai.opt_out_basis_hint')}
                  bind:value={basis}
                  options={[
                    { value: '', label: t('app.ai.opt_out_basis_choose') },
                    ...BASES.map((each) => ({ value: each, label: t(`app.ai.opt_out_basis_${each}`) })),
                  ]}
                  error={missing.basis && !basis ? t(missing.basis) : undefined}
                />
                <Input
                  label={t('app.ai.opt_out_contact')}
                  hint={t('app.ai.opt_out_contact_hint')}
                  maxlength={200}
                  bind:value={contact}
                />
              </Stack>
            </div>
            {#if isMovingToEverybody}
              <p class="consequence">{t('app.ai.opt_out_consequence', { count: source.keeping_out_count })}</p>
            {/if}
          {/if}
          {#if failure}<p class="failure" role="alert">{failure.message}</p>{/if}
        </Stack>
        <WriteOnly>
          <SaveBar
            label={t('app.save_bar.label')}
            changesLabel={changed.length > 0 ? t('app.save_bar.changes', { count: String(changed.length) }) : undefined}
            discardLabel={t('app.save_bar.discard')}
            saveLabel={t('app.ai.opt_out_save')}
            isSaving={isSaving}
            savingLabel={t('app.workspace.saving')}
            onDiscard={discard}
            onSave={() => form?.requestSubmit()}
          />
        </WriteOnly>
      </form>
    {/if}
  </Stack>
</section>
{/if}

<style>
  /* Prose and two fields: a measure of its own, as the form above it carries one. */
  .panel {
    max-inline-size: 72ch;
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    box-shadow: var(--shadow-raised);
  }

  .section { margin: 0; font-family: var(--font-display); font-size: var(--fs-300); font-weight: var(--fw-semibold); }

  p { margin: 0; }

  form { margin: 0; }

  .value {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--sp-100);
  }

  .quiet { color: var(--text-secondary); max-width: 64ch; }

  .fields { max-inline-size: 52ch; }

  .consequence { max-width: 64ch; }

  .failure { color: var(--text-danger); }
</style>
