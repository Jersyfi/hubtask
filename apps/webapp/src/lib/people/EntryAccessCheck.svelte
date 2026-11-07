<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // "May this person read, change or comment on this entry - and why?" at the entry itself
  // (UC-ID-26): the decision a request meets there, through a role, a share or the assignment.
  // The reader is chosen first, because anybody may ask about themselves; asking about somebody
  // else is the server's to allow, and its refusal is shown as one.
  import { Select, Spinner, Stack } from '@hubtask/design-system/components';
  import type { AccessExplanation } from '@hubtask/sync-engine';

  import { accounts } from '../data/accounts.svelte.ts';
  import { actor } from '../data/account.svelte.ts';
  import { roles } from '../data/roles.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem, type RenderedProblem } from '../problem.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { roleLabel } from './rolewords.ts';
  import { ENTRY_ACTIONS, entryVerdictCode } from './entryaccess.ts';

  type Props = {
    entryId: string;
    /** Who may be asked about: the people on the entry's path. */
    people: readonly string[];
  };
  const { entryId, people }: Props = $props();

  const me = $derived(actor.account?.id ?? '');
  let who = $state('');
  let action = $state<string>('READ');
  $effect(() => {
    if (who === '' && me !== '') who = me;
  });

  const personOptions = $derived(
    [...new Set([me, ...people].filter((id) => id !== ''))].map((id) => ({
      value: id,
      label: id === me ? t('app.access.entry_me') : (accounts.nameOf(id) ?? t('app.people.unnamed')),
    })),
  );
  const actionOptions = ENTRY_ACTIONS.map((value) => ({ value, label: t(`app.access.entry_action_${value.toLowerCase()}`) }));

  let answer = $state<AccessExplanation | undefined>(undefined);
  let failure = $state<RenderedProblem | undefined>(undefined);
  let isAsking = $state(false);

  $effect(() => {
    const person = who;
    const asked = action as (typeof ENTRY_ACTIONS)[number];
    const entry = entryId;
    if (!person) return;
    let isCurrent = true;
    isAsking = true;
    failure = undefined;
    roles
      .explainEntry(entry, asked, person === me ? undefined : person)
      .then((explained) => {
        if (isCurrent) answer = explained;
      })
      .catch((error: unknown) => {
        if (!isCurrent) return;
        answer = undefined;
        failure = renderProblem(error as never, messages);
      })
      .finally(() => {
        if (isCurrent) isAsking = false;
      });
    return () => {
      isCurrent = false;
    };
  });

  const name = $derived(who === me ? t('app.access.entry_you') : (accounts.nameOf(who) ?? t('app.people.unnamed')));
</script>

<Stack gap="100">
  <h3 class="section">{t('app.access.entry_title')}</h3>
  <Select label={t('app.access.person')} options={personOptions} bind:value={who} />
  <Select label={t('app.access.entry_action')} options={actionOptions} bind:value={action} />
  {#if isAsking}
    <p class="quiet"><Spinner label={t('app.access.asking')} /> {t('app.access.asking')}</p>
  {:else if failure}
    <FailureNote problem={failure} />
  {:else if answer?.entry}
    <p class="verdict" data-permitted={answer.entry.permitted}>
      {t(entryVerdictCode(answer.entry), { name })}
    </p>
    {#if answer.roles.length > 0}
      <p class="quiet">{t('app.access.roles_here')}: {answer.roles.map((role) => roleLabel(role)).join(', ')}</p>
    {/if}
  {/if}
</Stack>

<style>
  .section { margin: 0; font-size: var(--fs-100); }
  .verdict { margin: 0; }
  .quiet { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); }
</style>
