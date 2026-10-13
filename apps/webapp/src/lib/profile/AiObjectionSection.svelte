<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The person's own choice about AI on their profile (UC-PRV-05 checks 4, 7, 8, 9).
  //
  // **Drawn only where it means something.** Where AI does not answer the workspace, or the person
  // is alone in it, there is nothing to keep out from anybody and the section is absent (check 8).
  //
  // **One of two things, never both.** Where the workspace lets each person keep their content out
  // it is a switch; where it made AI part of everybody's work it is the sentence naming the basis
  // and whom to ask instead (check 9) - a switch the server would not honour would be a control
  // that lies (P-05).

  import { untrack } from 'svelte';

  import { Stack, Switch } from '@hubtask/design-system/components';

  import { aiObjection } from '../data/aiobjection.svelte.ts';
  import { announcer } from '../announce.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  $effect(() => untrack(() => aiObjection.open()));

  const position = $derived(aiObjection.position);

  let keepingOut = $state(false);
  let isSaving = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  $effect(() => {
    keepingOut = position?.keeping_out === true;
  });

  async function change(isOn: boolean) {
    isSaving = true;
    failure = undefined;
    try {
      if (isOn) {
        await aiObjection.keepOut();
      } else {
        await aiObjection.letIn();
      }
      announcer.say(t(isOn ? 'app.profile.ai_kept_out_announced' : 'app.profile.ai_let_in_announced'));
    } catch (error) {
      failure = renderProblem(error as never, messages);
      keepingOut = position?.keeping_out === true;
    } finally {
      isSaving = false;
    }
  }

  const contact = $derived(position?.contact || t('app.profile.ai_ask_admin'));
  const basis = $derived(position?.basis ? t(`app.profile.ai_basis_${position.basis}`) : '');
</script>

{#if position?.shown}
  <section class="panel" aria-labelledby="ai-objection-title" data-ai-objection>
    <Stack gap="200">
      <h2 id="ai-objection-title">{t('app.profile.ai_title')}</h2>
      {#if position.person_opt_out === 'NOT_OFFERED'}
        <p>{t('app.profile.ai_part_of_work', { basis, contact })}</p>
        {#if position.keeping_out}
          <p class="quiet">{t('app.profile.ai_choice_inactive')}</p>
        {/if}
      {:else}
        <Switch
          label={t('app.profile.ai_keep_out')}
          hint={t('app.profile.ai_keep_out_hint')}
          bind:checked={keepingOut}
          disabledReason={isSaving ? t('app.workspace.saving') : undefined}
          onchange={() => void change(keepingOut)}
        />
      {/if}
      {#if failure}<p class="failure" role="alert">{failure.message}</p>{/if}
    </Stack>
  </section>
{/if}

<style>
  .panel {
    max-inline-size: 60ch;
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }

  h2 { margin: 0; font-family: var(--font-display); font-size: var(--fs-300); font-weight: var(--fw-semibold); }

  p { margin: 0; }

  .quiet { color: var(--text-secondary); }

  .failure { color: var(--text-danger); }
</style>
