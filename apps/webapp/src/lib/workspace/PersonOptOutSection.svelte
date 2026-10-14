<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The workspace's position on people keeping their own content out of AI (UC-PRV-05 checks 9,
  // 11; data-protection.md §4.1), on its AI screen - the one place it is set (P-06).
  //
  // **Each person may keep their content out** is the default. **AI is part of everybody's work**
  // needs a legal basis the workspace names; the screen does not let it be saved without one, as
  // the server does not. Where the installation or a plan holds the workspace at the default, the
  // screen says who decided and offers no control (P-05, P-07).
  //
  // **Alone, nothing is asked** (P-10, UC-PRV-05 check 8): with one person there is nobody whose
  // content could be kept out, so the choice is not drawn. A stored *AI for everybody* stays
  // drawn, because it is still in force and only here can it be taken back.
  //
  // **How many people it touches is said before it is done** (P-04): a number, never who - the
  // consent register on *Data subject requests* is where names are.

  import { Button, Input, Radio, Select, Stack } from '@hubtask/design-system/components';
  import type { AiOptOutBasis, AiPersonOptOut, AiSource } from '@hubtask/sync-engine';

  import { aiProvider } from '../data/aiprovider.svelte.ts';
  import { workspace } from '../data/workspace.svelte.ts';
  import { announcer } from '../announce.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

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

  // Seeded from what the server holds, and again whenever it answers.
  $effect(() => {
    position = source.person_opt_out;
    basis = source.person_opt_out_basis ?? '';
    contact = source.person_opt_out_contact ?? '';
  });

  const isAsked = $derived(
    workspace.workspace?.several_people === true || source.person_opt_out === 'NOT_OFFERED',
  );
  const lockedBy = $derived(
    source.person_opt_out_lock
      ? source.person_opt_out_lock === 'PLAN'
        ? t('app.ai.set_by_plan')
        : t('app.ai.set_by_installation')
      : undefined,
  );
  const isMovingToEverybody = $derived(position === 'NOT_OFFERED' && source.person_opt_out !== 'NOT_OFFERED');
  const isChanged = $derived(
    position !== source.person_opt_out ||
      (position === 'NOT_OFFERED' &&
        (basis !== (source.person_opt_out_basis ?? '') || contact.trim() !== (source.person_opt_out_contact ?? ''))),
  );

  async function save() {
    isSaving = true;
    failure = undefined;
    try {
      await aiProvider.setPersonOptOut(
        position === 'NOT_OFFERED'
          ? { person_opt_out: 'NOT_OFFERED', basis: basis as AiOptOutBasis, contact: contact.trim() }
          : { person_opt_out: 'OFFERED' as AiPersonOptOut },
      );
      announcer.say(t('app.ai.opt_out_saved'));
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

    {#if lockedBy}
      <p>{t('app.ai.opt_out_locked', { by: lockedBy })}</p>
    {:else}
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
      <div>
        <Button
          tone="primary"
          isBusy={isSaving}
          busyLabel={t('app.workspace.saving')}
          disabledReason={!isChanged
            ? t('app.ai.opt_out_unchanged')
            : position === 'NOT_OFFERED' && !basis
              ? t('app.ai.opt_out_basis_needed')
              : undefined}
          onclick={() => void save()}
        >
          {t('app.ai.opt_out_save')}
        </Button>
      </div>
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
  }

  .section { margin: 0; font-family: var(--font-display); font-size: var(--fs-300); font-weight: var(--fw-semibold); }

  p { margin: 0; }

  .quiet { color: var(--text-secondary); max-width: 64ch; }

  .fields { max-inline-size: 52ch; }

  .consequence { max-width: 64ch; }

  .failure { color: var(--text-danger); }
</style>
