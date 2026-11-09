<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One sign-in provider as its record page (page-system §9.2): what it is, who may sign in
  // through it and what newcomers get, *Edit* - the provider form as a Dialog L - and its removal
  // in the head's ⋯ and the danger zone. It configures and switches nothing: whether it is a way
  // in here is set in the list of ways on *Sign-in*, the one place for it (UC-ID-11 check 8).
  //
  // An installation's provider is not this workspace's to change, so it carries no *Edit* and no
  // removal - only whose it is (UC-WRK-32 check 5). Where the plan leaves the workspace's own
  // providers out they rest: none is changed, removing one stays (multi-tenancy.md §4.2, P-05).
  import { untrack } from 'svelte';

  import { EmptyState, RecordPage } from '@hubtask/design-system/components';
  import type { IdentityProvider } from '@hubtask/sync-engine';

  import ProviderDialog from '../lib/signin/ProviderDialog.svelte';
  import RemoveProviderDialog from '../lib/signin/RemoveProviderDialog.svelte';
  import { certificateState } from '../lib/signin/certificate.ts';
  import { identityProvider } from '../lib/data/identityprovider.svelte.ts';
  import { groups } from '../lib/data/groups.svelte.ts';
  import { includedHere, mayHere } from '../lib/data/capability.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';

  interface Props {
    id: string;
    onnavigate: (path: string) => void;
  }

  const { id, onnavigate }: Props = $props();

  const LIST = '/administration/sign-in/ways';

  $effect(() => untrack(() => identityProvider.open()));
  $effect(() => untrack(() => groups.open()));

  const provider = $derived<IdentityProvider | undefined>(identityProvider.all.find((one) => one.id === id));
  const isRead = $derived(identityProvider.state.status === 'ready' || identityProvider.state.status === 'failed');
  const isOwn = $derived(provider?.scope === 'workspace');
  const mayManage = $derived(mayHere('MANAGE_MEMBERS').status === 'permitted');
  const resting = $derived(includedHere('OWN_SIGN_IN_PROVIDERS') === false);
  const name = $derived(provider?.display_name ?? t('app.identity_provider.title'));

  $effect(() => page.entitle(name));

  let isEditing = $state(false);
  let removing = $state<{ id: string; name: string } | undefined>(undefined);

  const status = $derived.by(() => {
    if (!provider) return undefined;
    if (isOwn && resting) return { label: t('app.plan.resting'), tone: 'neutral' as const };
    if (!provider.enabled) return { label: t('app.identity_provider.badge_off'), tone: 'warning' as const };
    return provider.offered_here
      ? { label: t('app.identity_provider.badge_on_here'), tone: 'success' as const }
      : { label: t('app.identity_provider.badge_off_here'), tone: 'neutral' as const };
  });

  /** What newcomers get, in the words the form offers it. */
  const newcomers = $derived.by(() => {
    if (!provider) return '';
    if (provider.newcomer_role) {
      return t('app.identity_provider.newcomers_role', { role: t(`app.people.role.${provider.newcomer_role.toLowerCase()}`) });
    }
    if (provider.newcomer_group_id) {
      return t('app.identity_provider.newcomers_group', { group: groups.nameOf(provider.newcomer_group_id) ?? t('app.people.unnamed_group') });
    }
    return t('app.identity_provider.newcomers_nothing');
  });

  const certificateLine = $derived.by(() => {
    if (!provider?.saml_certificate_expires_at) return undefined;
    const date = formatDateTime(provider.saml_certificate_expires_at, messages.locale);
    const state = certificateState(provider.saml_certificate_expires_at, Date.now());
    return state === 'ended'
      ? t('app.identity_provider.certificate_ended', { date })
      : t('app.identity_provider.certificate_ending', { date });
  });

  const facts = $derived(
    provider
      ? {
          title: t('app.identity_provider.facts_title'),
          rows: [
            { label: t('app.identity_provider.issuer_label'), value: provider.issuer },
            { label: t('app.identity_provider.column_from'), value: isOwn ? t('app.identity_provider.from_workspace') : t('app.identity_provider.badge_inherited') },
            { label: t('app.identity_provider.provisioning_label'), value: t(`app.identity_provider.provisioning.${provider.provisioning.toLowerCase()}`) },
            ...(provider.provisioning !== 'INVITED_ONLY' ? [{ label: t('app.identity_provider.newcomers_label'), value: newcomers }] : []),
            ...(provider.allowed_email_domains.length > 0 ? [{ label: t('app.identity_provider.domains_label'), value: provider.allowed_email_domains.join(', ') }] : []),
            ...(certificateLine ? [{ label: t('app.identity_provider.saml_certificate_label'), value: certificateLine }] : []),
          ],
        }
      : undefined,
  );

  const mayRemove = $derived(provider !== undefined && isOwn && mayManage);
</script>

<RecordPage
  record={{
    kind: t('app.identity_provider.kind_record'),
    title: name,
    back: { label: t('app.signin_settings.title'), href: LIST, onnavigate: () => onnavigate(LIST) },
    status,
    facts: provider?.issuer,
    edit: provider && isOwn && mayManage && !resting ? { label: t('app.page.edit'), opener: 'edit-provider', onclick: () => (isEditing = true) } : undefined,
    menu: mayRemove && provider
      ? {
          label: t('app.page.more_for', { name }),
          items: [{ id: 'remove', label: t('app.identity_provider.remove_more'), isDestructive: true }],
          onselect: () => (removing = { id: provider.id, name: provider.display_name }),
        }
      : undefined,
  }}
  {facts}
  danger={mayRemove && provider
    ? {
        title: t('app.page.danger_zone'),
        acts: [{ id: 'remove', title: t('app.identity_provider.remove_zone'), cost: t('app.identity_provider.remove_cost'), actionLabel: t('app.identity_provider.remove_more') }],
        onact: () => (removing = { id: provider.id, name: provider.display_name }),
      }
    : undefined}
  refused={isRead && !provider ? notHere : undefined}
/>

{#snippet notHere()}
  <EmptyState kind="unused" title={t('app.identity_provider.gone')} />
{/snippet}

{#if provider && isOwn}
  <ProviderDialog {provider} bind:isOpen={isEditing} />
{/if}
<RemoveProviderDialog target={removing} onClose={() => (removing = undefined)} onremoved={() => onnavigate(LIST)} />
