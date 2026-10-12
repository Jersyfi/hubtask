<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Add someone without an address* (UC-ID-20 checks 1 and 2): a child, a grandparent, a shift
  // worker gets an account with a sign-in name, and a role at the workspace or at one hub.
  //
  // **Nobody chooses the start password.** The server draws it and answers it once; it is shown
  // here in `OneTimeSecret`, to copy, and on a slip to print. Nothing keeps it: the answer is held
  // in this component only until the reader dismisses it, and the slip exists in the document only
  // while the browser prints it - a credential is in the DOM no longer than its one showing.

  import { tick } from 'svelte';

  import { Banner, Button, Input, OneTimeSecret, Select, Stack } from '@hubtask/design-system/components';
  import type { MembershipRole } from '@hubtask/sync-engine';

  import { containers } from '../data/containers.svelte.ts';
  import { people } from '../data/people.svelte.ts';
  import { announcer } from '../announce.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    /** The roles the installation reports, already labelled. */
    roleOptions: readonly { value: string; label: string }[];
  }

  const { roleOptions }: Props = $props();

  /** The workspace itself, or one hub - what a person is given a place in from this screen. */
  const WORKSPACE = '';

  let signInName = $state('');
  let displayName = $state('');
  let role = $state('');
  let where = $state(WORKSPACE);
  let isWriting = $state(false);
  let failure = $state<string | undefined>(undefined);
  let nameError = $state<string | undefined>(undefined);
  /** The one answer that carried the start password, until the reader dismisses it. */
  let created = $state<{ signInName: string; startPassword: string } | undefined>(undefined);
  /** True only while the browser prints the slip. */
  let printing = $state(false);

  const whereOptions = $derived([
    { value: WORKSPACE, label: t('app.people.managed_where_workspace') },
    ...containers.hubs.map((hub) => ({ value: hub.id, label: hub.name })),
  ]);

  async function create(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    if (!signInName.trim() || !displayName.trim() || !role) return;
    failure = undefined;
    nameError = undefined;
    isWriting = true;
    try {
      created = await people.createManaged(
        signInName.trim(),
        displayName.trim(),
        role as MembershipRole,
        where === WORKSPACE ? { scopeType: 'TENANT' } : { scopeType: 'HUB', scopeId: where },
      );
      announcer.say(t('app.people.managed_created_announced'));
      signInName = '';
      displayName = '';
      role = '';
      where = WORKSPACE;
    } catch (error) {
      const problem = renderProblem(error as never, messages);
      // A refusal of the name belongs under the name's field; anything else above the form.
      const own = problem.fields.get('/sign_in_name');
      if (own) nameError = own;
      else failure = problem.message;
    } finally {
      isWriting = false;
    }
  }

  async function print(): Promise<void> {
    printing = true;
    await tick();
    window.print();
    printing = false;
  }
</script>

<Stack gap="150">
  <h2 class="section">{t('app.people.managed_title')}</h2>
  <p class="quiet small">{t('app.people.managed_hint')}</p>

  {#if failure}
    <Banner tone="danger">{failure}</Banner>
  {/if}

  {#if created}
    <!-- The only time the start password exists anywhere but the server's hash. -->
    <OneTimeSecret
      value={created.startPassword}
      label={t('app.people.managed_start_password', { name: created.signInName })}
      hint={t('app.people.managed_start_password_hint')}
      revealLabel={t('app.people.managed_reveal')}
      hideLabel={t('app.people.managed_hide')}
      copyLabel={t('app.people.managed_copy')}
      copiedLabel={t('app.people.managed_copied')}
      acknowledgementLabel={t('app.people.managed_kept')}
      notAcknowledgedReason={t('app.people.managed_keep_first')}
      dismissLabel={t('app.tokens.done')}
      onDismiss={() => (created = undefined)}
    />
    <div>
      <Button tone="secondary" onclick={() => void print()}>{t('app.people.managed_print')}</Button>
    </div>
    {#if printing}
      <!-- What a person takes to the child: where, with what name, with which password, and that
           they choose their own at once. Drawn for the printer only. -->
      <div class="slip" data-slip>
        <p class="slip-title">{t('app.people.managed_slip_title')}</p>
        <dl>
          <dt>{t('app.people.managed_slip_address')}</dt>
          <dd>{location.host}</dd>
          <dt>{t('app.people.managed_slip_name')}</dt>
          <dd>{created.signInName}</dd>
          <dt>{t('app.people.managed_slip_password')}</dt>
          <dd class="code">{created.startPassword}</dd>
        </dl>
        <p>{t('app.people.managed_slip_next')}</p>
      </div>
    {/if}
  {/if}

  <form class="panel" onsubmit={create}>
    <Stack gap="150">
      <Input
        label={t('app.people.managed_name')}
        hint={t('app.people.managed_name_hint')}
        bind:value={signInName}
        error={nameError}
        autocomplete="off"
        autocapitalize="none"
        spellcheck={false}
        isRequired
      />
      <Input
        label={t('app.people.invite_name')}
        hint={t('app.people.managed_display_hint')}
        bind:value={displayName}
        autocomplete="off"
        isRequired
      />
      <Select
        label={t('app.people.role_column')}
        bind:value={role}
        placeholder={t('app.people.choose_role')}
        options={roleOptions}
      />
      <Select
        label={t('app.people.managed_where')}
        bind:value={where}
        options={whereOptions}
      />
      <div>
        <Button type="submit" tone="primary" isBusy={isWriting} busyLabel={t('app.people.working')}>
          {t('app.people.managed_submit')}
        </Button>
      </div>
    </Stack>
  </form>
</Stack>

<style>
  form { margin: 0; max-inline-size: 52ch; }

  .panel {
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }

  .section { margin: 0; font-family: var(--font-display); font-size: var(--fs-300); font-weight: var(--fw-semibold); }

  .quiet { margin: 0; color: var(--text-secondary); }

  .small { font-size: var(--fs-075); }

  /* The slip is for paper only: on screen it is never drawn. */
  .slip { display: none; }

  @media print {
    /* While the slip is printed, it is the page: everything else stays off the paper. The rule
       reaches outside this component on purpose - in print only, and only while a slip exists, so
       printing this screen at any other moment prints the screen. */
    :global(body:has([data-slip]) *) { visibility: hidden; }

    .slip,
    .slip * { visibility: visible; }

    .slip {
      display: block;
      position: absolute;
      inset-block-start: 0;
      inset-inline-start: 0;
      padding: var(--sp-300);
      font-size: var(--fs-300);
      color: var(--text-primary);
    }

    .slip-title { font-family: var(--font-display); font-weight: var(--fw-semibold); }

    .code { font-family: var(--font-mono); }
  }
</style>
