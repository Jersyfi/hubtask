<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A sign-in provider added or changed (UC-ID-11): the one provider form (`ProviderFields`) and
  // what newcomers get, as a Dialog L - up to twelve fields in a few groups (page-system §5.1).
  //
  // **The client secret goes one way.** It is sealed on the way in and is a member of no answer, so
  // changing a provider shows an empty secret field whose hint says leaving it empty keeps the one
  // sealed; the secret leaves the component's state the moment it is sent.
  //
  // **No switch here**: a provider is turned on or off in the list of ways to sign in, and nowhere
  // else (UC-ID-11 check 8). A new one is saved off.
  //
  // **What newcomers get is asked only where somebody new arrives**: under the two modes that
  // create accounts (UC-ID-11 check 5). The roles come from the manifest and never include OWNER,
  // which the server refuses for newcomers; the groups are the workspace's own.
  import { tick, untrack } from 'svelte';

  import { Button, Dialog, Select, Stack } from '@hubtask/design-system/components';
  import type { IdentityProvider, IdentityProviderConfiguration } from '@hubtask/sync-engine';
  import { TransportError } from '@hubtask/sync-engine';

  import ProviderFields from './ProviderFields.svelte';
  import { draftBody, draftOf, draftRefusal, emptyDraft } from './issuer.ts';
  import { identityProvider } from '../data/identityprovider.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { groups } from '../data/groups.svelte.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { toast } from '../frame/toasts.svelte.ts';
  import { focusFirst } from '../forms/missing.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import DiscardDialog from '../page/DiscardDialog.svelte';
  import { DiscardGuard } from '../page/discard.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    /** The provider being changed; none adds one. */
    provider?: IdentityProvider;
    isOpen?: boolean;
    /** Called with the provider the server answered. */
    onsaved?: (provider: IdentityProvider) => void;
  }

  let { provider, isOpen = $bindable(false), onsaved }: Props = $props();

  let draft = $state(emptyDraft('DOMAINS'));
  /** What newcomers get: empty is nothing, `role:<ROLE>` or `group:<id>` one of the two. */
  let newcomers = $state('');
  /** The order the buttons are drawn in: kept, not shown - a new one goes last. */
  let position = $state(0);
  let isWorking = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let form = $state<HTMLFormElement | undefined>(undefined);
  /** What the form held when it opened, for the discard question. */
  let opened = '';

  $effect(() => untrack(() => groups.open()));

  const snapshot = (): string => JSON.stringify({ draft, newcomers });

  // Filled each time it opens, from the provider it is about: a second opening starts from what
  // is saved, never from what the last one left.
  $effect(() => {
    if (!isOpen) return;
    untrack(() => {
      draft = provider ? draftOf(provider) : emptyDraft('DOMAINS');
      newcomers = provider?.newcomer_role
        ? `role:${provider.newcomer_role}`
        : provider?.newcomer_group_id
          ? `group:${provider.newcomer_group_id}`
          : '';
      position = provider ? provider.position : identityProvider.own.length;
      failure = undefined;
      opened = snapshot();
    });
  });

  function close(): void {
    draft.clientSecret = '';
    isOpen = false;
  }

  const guard = new DiscardGuard(() => snapshot() !== opened, close);

  const preset = $derived(identityProvider.presetOf(draft.kind as IdentityProvider['kind']));
  const createsAccounts = $derived(draft.provisioning === 'DOMAINS' || draft.provisioning === 'ANY');

  const newcomerOptions = $derived([
    { value: '', label: t('app.identity_provider.newcomers_nothing') },
    ...(manifest.value?.roles ?? [])
      .map((description) => description.role)
      .filter((role): role is NonNullable<typeof role> => role !== undefined && role !== 'OWNER')
      .map((role) => ({
        value: `role:${role}`,
        label: t('app.identity_provider.newcomers_role', { role: t(`app.people.role.${role.toLowerCase()}`) }),
      })),
    ...groups.all.map((group) => ({
      value: `group:${group.id}`,
      label: t('app.identity_provider.newcomers_group', { group: group.name }),
    })),
  ]);

  async function save(): Promise<void> {
    // Checked here rather than switching Save off: the field already says why the server would
    // refuse it; pressed, focus goes there and nothing is sent (UC-WRK-32 check 3).
    if (draftRefusal(draft, preset) !== undefined) {
      const target = form;
      void tick().then(() => target && focusFirst(target));
      return;
    }
    isWorking = true;
    failure = undefined;
    // No `enabled`: the list of ways to sign in is the one place a way in is turned on or off
    // (UC-ID-11 check 8, ADR-0076 §5).
    const body: IdentityProviderConfiguration = { ...draftBody(draft, preset), position };
    // Sent only where somebody new arrives: the server refuses a newcomer setting under a mode that
    // creates nobody, and the configuration is set whole, so leaving it out is "nothing".
    if (createsAccounts && newcomers.startsWith('role:')) {
      body.newcomer_role = newcomers.slice('role:'.length) as NonNullable<IdentityProviderConfiguration['newcomer_role']>;
    } else if (createsAccounts && newcomers.startsWith('group:')) {
      body.newcomer_group_id = newcomers.slice('group:'.length);
    }
    try {
      const answer = provider ? await identityProvider.configure(provider.id, body) : await identityProvider.add(body);
      draft.clientSecret = '';
      isOpen = false;
      toast.show({ text: t('app.identity_provider.saved'), tone: 'success' });
      if (answer) onsaved?.(answer as IdentityProvider);
    } catch (cause) {
      failure = cause instanceof TransportError
        ? renderProblem(cause, messages)
        : { message: messages.t('app.something_went_wrong'), fields: new Map(), isServerFault: true };
    } finally {
      isWorking = false;
    }
  }
</script>

<Dialog
  size="l"
  bind:isOpen
  title={provider ? t('app.identity_provider.edit_title') : t('app.identity_provider.add_title')}
  dismissLabel={t('app.identity_provider.cancel')}
  onDismiss={guard.dismiss}
  onClose={() => (draft.clientSecret = '')}
  primary={{ label: t('app.identity_provider.save'), onclick: () => void save(), isBusy: isWorking, busyLabel: t('app.identity_provider.saving') }}
>
  {#if isOpen}
    <form bind:this={form} onsubmit={(event) => { event.preventDefault(); void save(); }}>
      <Stack gap="200">
        <ProviderFields bind:draft presets={identityProvider.presets} level="workspace" isNew={provider === undefined}>
          {#snippet more()}
            {#if createsAccounts}
              <Select
                label={t('app.identity_provider.newcomers_label')}
                hint={t('app.identity_provider.newcomers_hint')}
                options={newcomerOptions}
                bind:value={newcomers}
              />
            {/if}
          {/snippet}
        </ProviderFields>
        <p class="quiet">{t('app.identity_provider.switched_in_sign_in')}</p>
        {#if failure}
          <!-- The server's own sentence: an issuer that could not be reached, a mark on an issuer
               it does not belong to, a mode the provider may not use. -->
          <FailureNote problem={failure} />
        {/if}
      </Stack>
    </form>
  {/if}
  {#snippet actions()}
    <Button tone="secondary" onclick={() => { if (guard.dismiss()) close(); }}>{t('app.identity_provider.cancel')}</Button>
  {/snippet}
</Dialog>
<DiscardDialog {guard} />

<style>
  form { margin: 0; }
  .quiet { margin: 0; color: var(--text-secondary); }
</style>
