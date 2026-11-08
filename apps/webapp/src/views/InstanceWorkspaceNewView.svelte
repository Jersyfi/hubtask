<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *New workspace* at the installation as a create page (page-system §7.7, §9.4): the workspace -
  // its address, name and plan - and its owner as two sections of setting rows, *Create it* the
  // head's primary. A workspace, and the person who owns it: nobody else is added.
  //
  // **The owner's way in is shown once, here.** Where the installation sends mail the invitation is
  // queued and nothing is shown, since the mail's own way in voids any other (UC-INS-07 check 3);
  // otherwise the redemption token stands on this page, through `OneTimeSecret`, until it is left.
  // It is held by the page, never by a store, so the record page it leads to cannot show it again.

  import { tick, untrack } from 'svelte';

  import { Banner, CreatePage, FormLayout, FormSection, Input, OneTimeSecret, Select, SettingRow } from '@hubtask/design-system/components';
  import type { ProvisionedTenant } from '@hubtask/sync-engine';
  import { TransportError } from '@hubtask/sync-engine';

  import InstanceGate from '../lib/instance/InstanceGate.svelte';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { instance } from '../lib/data/instance.svelte.ts';
  import { plans } from '../lib/data/plans.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { installationTrail } from '../lib/instance/trail.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { focusFirst } from '../lib/forms/missing.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import FailureNote from '../lib/frame/FailureNote.svelte';

  interface Props {
    onnavigate: (path: string) => void;
  }

  const { onnavigate }: Props = $props();

  const LIST = '/instance/workspaces';

  let slug = $state('');
  /** The host a new workspace answers at, where the installation assigns hosts (ADR-0095 §6). */
  let host = $state('');
  let displayName = $state('');
  let ownerEmail = $state('');
  let ownerName = $state('');
  /** The plan a new workspace starts on; empty for none. */
  let newPlan = $state('');
  /** Whether the plan field was preset yet: the plans may arrive after the page opened. */
  let isPlanPreset = $state(false);
  let isWorking = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  /** The owner's way in, from the one answer that carries it. */
  let provisioned = $state<ProvisionedTenant | undefined>(undefined);
  /** The address the owner's invitation was mailed to, said beside the answer that has none. */
  let mailedTo = $state('');

  $effect(() => {
    if (!instance.isElevated) return;
    const stops = untrack(() => [instance.openWorkspaces(), ...(manifest.isMultiTenant ? [plans.open()] : [])]);
    return () => stops.forEach((stop) => stop());
  });
  const hasPlans = $derived(manifest.isMultiTenant && plans.all.length > 0);
  // One plan is the plan a new workspace starts on, unless the operator says otherwise.
  $effect(() => {
    if (isPlanPreset || plans.state.status !== 'ready') return;
    isPlanPreset = true;
    newPlan = untrack(() => (plans.all.length === 1 ? (plans.all[0]?.name ?? '') : ''));
  });

  // Whether a workspace already answers at the short name, a moment after the last keystroke: one
  // read of the query, not the whole list.
  let slugTaken = $state(false);
  $effect(() => {
    const typed = slug.trim();
    if (typed === '') {
      slugTaken = false;
      return;
    }
    let current = true;
    const timer = setTimeout(() => {
      void instance.isSlugTaken(typed).then((taken) => { if (current) slugTaken = taken; }, () => {});
    }, 300);
    return () => {
      current = false;
      clearTimeout(timer);
    };
  });
  const typedHost = $derived(host.trim().toLowerCase().replace(/\.$/, ''));
  const hostTaken = $derived(
    manifest.hasAssignedHosts && typedHost !== '' && instance.workspaces.some((each) => each.host === typedHost),
  );
  /** The short name the server derives from the host where none is typed: shown as the placeholder. */
  const shortNameOfHost = $derived.by(() => {
    if (!/^[^\s/:@?#*_]+\.[^\s/:@?#*_]+$/.test(typedHost)) return '';
    let ascii = '';
    try {
      ascii = typedHost === '' ? '' : new URL(`http://${typedHost}`).hostname;
    } catch {
      return '';
    }
    let name = '';
    for (const label of ascii.split('.')) {
      name = name === '' ? label : `${name}-${label}`;
      const made = name.replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 40).replace(/-+$/, '');
      if (made.length >= 3) return made;
    }
    return '';
  });

  async function create(): Promise<void> {
    // A taken address is said at its field; pressed, focus goes there and nothing is sent
    // (UC-WRK-32 check 3) - never a switched-off button repeating the field.
    if (slugTaken || hostTaken) {
      await tick();
      focusFirst(document);
      return;
    }
    isWorking = true;
    failure = undefined;
    try {
      mailedTo = ownerEmail.trim();
      provisioned = await instance.provision({
        ...(manifest.hasAssignedHosts ? { host: host.trim() } : {}),
        ...(slug.trim() === '' ? {} : { slug: slug.trim() }),
        display_name: displayName.trim(),
        owner_email: ownerEmail.trim(),
        ...(ownerName.trim() === '' ? {} : { owner_display_name: ownerName.trim() }),
        ...(newPlan === '' ? {} : { plan: newPlan }),
      });
    } catch (cause) {
      failure =
        cause instanceof TransportError
          ? renderProblem(cause, messages)
          : { message: messages.t('app.something_went_wrong'), fields: new Map(), isServerFault: true };
    } finally {
      isWorking = false;
    }
  }

  const title = $derived(t('app.instance.workspace_new'));
  $effect(() => page.entitle(title));
  const opened = $derived(provisioned ? `${LIST}/${provisioned.id}` : LIST);
</script>

<InstanceGate onleave={() => onnavigate('/')}>
  {#snippet children(door)}
    <CreatePage
      head={{
        title,
        subtitle: t('app.instance.workspace_new_intro'),
        breadcrumb: installationTrail({ id: 'workspaces', label: t('app.instance.workspaces'), href: LIST }, { id: 'screen', label: title }),
        primary: !manifest.isMultiTenant
          ? undefined
          : provisioned
            ? { label: t('app.instance.workspace_open'), onclick: () => onnavigate(opened) }
            : { label: isWorking ? t('app.instance.working') : t('app.instance.workspace_create'), onclick: () => { if (!isWorking) void create(); } },
        secondary: provisioned ? [] : [{ label: t('app.instance.cancel'), isQuiet: true, onclick: () => onnavigate(LIST) }],
      }}
      close={{ label: t('app.instance.cancel'), onclose: () => onnavigate(LIST) }}
      refused={door}
    >
      {#if provisioned?.owner_delivery === 'MAIL'}
        <Banner tone="success" title={t('app.instance.owner_mailed_title')}>
          {t('app.instance.owner_mailed', { email: mailedTo })}
        </Banner>
      {:else if provisioned?.owner_redemption_token}
        <OneTimeSecret
          value={provisioned.owner_redemption_token}
          label={t('app.instance.owner_token_title')}
          hint={t('app.instance.owner_token_hint')}
          revealLabel={t('app.instance.reveal')}
          hideLabel={t('app.instance.hide')}
          copyLabel={t('app.instance.copy')}
          copiedLabel={t('app.instance.copied')}
          acknowledgementLabel={t('app.instance.owner_token_kept')}
          notAcknowledgedReason={t('app.instance.owner_token_keep_first')}
          dismissLabel={t('app.instance.workspace_open')}
          onDismiss={() => onnavigate(opened)}
        />
      {:else}
        {#if failure && failure.fields.size === 0}<FailureNote problem={failure} />{/if}
        <form onsubmit={(event) => { event.preventDefault(); void create(); }}>
          <FormLayout place="page">
            <FormSection title={t('app.instance.workspace_section')}>
              {#if manifest.hasAssignedHosts}
                <SettingRow label={t('app.instance.host_label')} help={t('app.instance.host_hint')} fieldWidth="lg" isRequired>
                  {#snippet control(nameId)}
                    <Input
                      label={t('app.instance.host_label')}
                      isLabelHidden
                      aria-labelledby={nameId}
                      error={hostTaken ? t('app.instance.host_taken', { host: typedHost }) : failure?.fields.get('/host')}
                      bind:value={host}
                      face="code"
                      autocomplete="off"
                      spellcheck={false}
                      isRequired
                    />
                  {/snippet}
                </SettingRow>
                <SettingRow label={t('app.instance.short_name_label')} help={t('app.instance.short_name_hint')} fieldWidth="md">
                  {#snippet control(nameId)}
                    <Input
                      label={t('app.instance.short_name_label')}
                      isLabelHidden
                      aria-labelledby={nameId}
                      error={slugTaken ? t('app.instance.slug_taken', { slug: slug.trim() }) : failure?.fields.get('/slug')}
                      bind:value={slug}
                      placeholder={shortNameOfHost}
                      face="code"
                      autocomplete="off"
                      spellcheck={false}
                    />
                  {/snippet}
                </SettingRow>
              {:else}
                <SettingRow label={t('app.instance.slug_label')} help={t('app.instance.slug_hint')} fieldWidth="md" isRequired>
                  {#snippet control(nameId)}
                    <Input
                      label={t('app.instance.slug_label')}
                      isLabelHidden
                      aria-labelledby={nameId}
                      error={slugTaken ? t('app.instance.slug_taken', { slug: slug.trim() }) : failure?.fields.get('/slug')}
                      bind:value={slug}
                      autocomplete="off"
                      spellcheck={false}
                      isRequired
                    />
                  {/snippet}
                </SettingRow>
              {/if}
              <SettingRow label={t('app.instance.name_label')} fieldWidth="md" isRequired>
                {#snippet control(nameId)}
                  <Input label={t('app.instance.name_label')} isLabelHidden aria-labelledby={nameId} bind:value={displayName} error={failure?.fields.get('/display_name')} autocomplete="off" isRequired />
                {/snippet}
              </SettingRow>
              {#if hasPlans}
                <SettingRow label={t('app.instance.new_plan_label')} help={t('app.instance.new_plan_hint')} fieldWidth="md">
                  {#snippet control(nameId)}
                    <Select
                      label={t('app.instance.new_plan_label')}
                      isLabelHidden
                      aria-labelledby={nameId}
                      options={[{ value: '', label: t('app.instance.plan_none') }, ...plans.all.map((plan) => ({ value: plan.name, label: plan.display_name }))]}
                      bind:value={newPlan}
                      error={failure?.fields.get('/plan')}
                    />
                  {/snippet}
                </SettingRow>
              {/if}
            </FormSection>
            <FormSection title={t('app.instance.owner_section')}>
              <SettingRow label={t('app.instance.owner_email_label')} help={t('app.instance.owner_email_hint')} fieldWidth="lg" isRequired>
                {#snippet control(nameId)}
                  <Input label={t('app.instance.owner_email_label')} isLabelHidden aria-labelledby={nameId} bind:value={ownerEmail} error={failure?.fields.get('/owner_email')} type="email" autocomplete="off" isRequired />
                {/snippet}
              </SettingRow>
              <SettingRow label={t('app.instance.owner_name_label')} fieldWidth="md">
                {#snippet control(nameId)}
                  <Input label={t('app.instance.owner_name_label')} isLabelHidden aria-labelledby={nameId} bind:value={ownerName} autocomplete="off" />
                {/snippet}
              </SettingRow>
            </FormSection>
          </FormLayout>
        </form>
      {/if}
    </CreatePage>
  {/snippet}
</InstanceGate>

<style>
  form { margin: 0; }
</style>
