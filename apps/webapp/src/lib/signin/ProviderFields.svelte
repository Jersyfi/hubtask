<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The one provider form, for a workspace's own providers and for the ones the installation offers
  // every workspace: which provider, the registration, issuer, name, client, secret, who it admits
  // and the list it is admitted on. One form, so a field reads, refuses and saves alike at both
  // levels. What only one level asks - newcomers at the workspace, the position at the installation
  // - comes in as `more`, after the admission it belongs to.
  //
  // **The modes come from the server, per level.** Offered to every workspace, a preset without a
  // directory claim may admit only the invited (ADR-0071's addendum); the preset answers both lists,
  // so the select offers what would be accepted, and a mode the kind does not permit is corrected to
  // the strictest the moment it changes - a rule rather than a change handler, because the value
  // also arrives from a stored row.
  //
  // **The secret goes one way.** Sealed on the way in and in no answer, so a stored provider shows an
  // empty field that says leaving it empty keeps the sealed one.

  import type { Snippet } from 'svelte';

  import { Banner, Input, Select, Stack, Textarea } from '@hubtask/design-system/components';
  import type { IdentityProviderPreset } from '@hubtask/sync-engine';

  import RedirectAddress from './RedirectAddress.svelte';
  import {
    admissionLists,
    draftRefusal,
    isPersonalIssuer,
    isSharedIssuer,
    issuerAfterChoosing,
    isSaml,
    offeredModes,
    type ProviderDraft,
  } from './issuer.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';

  interface Props {
    draft: ProviderDraft;
    presets: readonly IdentityProviderPreset[];
    /** Whose provider it is, which decides the modes a preset permits. */
    level: 'workspace' | 'installation';
    /** A new provider has no sealed secret to keep. */
    isNew: boolean;
    more?: Snippet;
  }

  let { draft = $bindable(), presets, level, isNew, more }: Props = $props();

  const preset = $derived(presets.find((each) => each.kind === draft.kind));
  const modes = $derived(
    preset === undefined ? [] : level === 'installation' ? preset.installation_provisioning : preset.provisioning,
  );
  const shared = $derived(isSharedIssuer(preset, draft.issuer));
  const lists = $derived(admissionLists(preset, draft.provisioning, shared));
  const personal = $derived(isPersonalIssuer(preset, draft.issuer));
  /** Said on the field as it is typed, and the reason the screen's *Save* waits. */
  const refusal = $derived.by(() => {
    const directory = draftRefusal(draft, preset);
    return directory === undefined ? undefined : t('identity_provider.directory_personal', { directory });
  });
  /** Where the level narrows this preset to the invited, which the form says beside the one answer. */
  const narrowedToInvited = $derived(preset !== undefined && modes.length === 1 && preset.provisioning.length > 1);

  $effect(() => {
    const strictest = modes[0];
    if (strictest === undefined) return;
    if (!modes.includes(draft.provisioning as (typeof modes)[number])) draft.provisioning = strictest;
  });
</script>

<Stack gap="200">
  <Select
    label={t('app.identity_provider.kind_label')}
    hint={t('app.identity_provider.kind_hint')}
    options={presets
      .filter((each) => level === 'workspace' || each.installation_provisioning.length > 0)
      .map((each) => ({ value: each.kind, label: t(`app.identity_provider.kind.${each.kind.toLowerCase()}`) }))}
    bind:value={draft.kind}
    onchange={(event) => {
      const chosen = (event.currentTarget as HTMLSelectElement).value;
      draft.issuer = issuerAfterChoosing(presets, chosen, draft.issuer);
      // A SAML provider's client ID is this installation's entity ID, its metadata address unless
      // the provider wants another.
      const metadata = presets.find((each) => each.kind === chosen)?.metadata_url;
      if (metadata && draft.clientId.trim() === '') draft.clientId = metadata;
    }}
  />
  {#if preset}
    <!-- The server's message code rendered with this installation's own callback: the one value
         every registration form at every provider asks for. -->
    <Banner tone="info" title={t('app.identity_provider.registration')}>
      {messages.t(preset.instructions, { redirect_uri: preset.redirect_uri })}
      {#if preset.particular}
        {' '}{messages.t(preset.particular, { redirect_uri: preset.redirect_uri })}
      {/if}
    </Banner>
    <RedirectAddress address={preset.redirect_uri} />
    {#if preset.metadata_url}
      <!-- A SAML provider is registered from this installation's metadata; its address is also
           the entity ID the client ID suggests. -->
      <RedirectAddress address={preset.metadata_url} />
    {/if}
  {/if}
  <Input
    label={t('app.identity_provider.issuer_label')}
    hint={t('app.identity_provider.issuer_hint')}
    bind:value={draft.issuer}
    type="url"
    autocomplete="off"
    spellcheck={false}
    isRequired
  />
  {#if preset?.supports_templated_issuer}
    <!-- Said where the issuer is typed: the shared endpoints make one registration serve many
         organisations, and they are also what needs the directories named. -->
    <Banner tone="info">{t('app.identity_provider.shared_endpoint')}</Banner>
  {/if}
  <Input
    label={t('app.identity_provider.name_label')}
    hint={t('app.identity_provider.name_hint')}
    bind:value={draft.displayName}
    autocomplete="off"
  />
  <Input
    label={t('app.identity_provider.client_id_label')}
    hint={t('app.identity_provider.client_id_hint')}
    bind:value={draft.clientId}
    autocomplete="off"
    spellcheck={false}
    isRequired
  />
  {#if isSaml(draft)}
    <!-- A SAML provider holds no secret: its single sign-on address and the certificates its
         assertions are signed with take the secret's place (identity.md §26). -->
    <Input
      label={t('app.identity_provider.saml_sso_url_label')}
      hint={t('app.identity_provider.saml_sso_url_hint')}
      bind:value={draft.samlSsoUrl}
      type="url"
      autocomplete="off"
      spellcheck={false}
      isRequired
    />
    <Textarea
      label={t('app.identity_provider.saml_certificate_label')}
      hint={t('app.identity_provider.saml_certificate_hint')}
      bind:value={draft.samlCertificate}
      rows={6}
      spellcheck={false}
      isRequired
    />
  {:else}
    <Input
      label={t('app.identity_provider.client_secret_label')}
      hint={isNew ? t('app.identity_provider.client_secret_hint') : t('app.identity_provider.client_secret_kept')}
      bind:value={draft.clientSecret}
      type="password"
      autocomplete="off"
      spellcheck={false}
      isRequired={isNew}
    />
  {/if}
  <Select
    label={t('app.identity_provider.provisioning_label')}
    hint={t('app.identity_provider.provisioning_hint')}
    options={offeredModes(modes, personal, draft.provisioning).map((mode) => ({
      value: mode,
      label: t(`app.identity_provider.provisioning.${mode.toLowerCase()}`),
    }))}
    bind:value={draft.provisioning}
  />
  {#if narrowedToInvited}
    <Banner tone="info">{t('app.instance.provider_invited_only')}</Banner>
  {/if}
  {#if personal && draft.provisioning === 'ANY'}
    <!-- The save refuses ANY on this issuer; the sentence names what may be chosen instead. Not
         moved for the person - a row stored so still admits organisation accounts. -->
    <Banner tone="info">{t('identity_provider.provisioning_personal', { issuer: draft.issuer.trim() })}</Banner>
  {/if}
  <!-- The list the server will read, and only that one (ADR-0071 §2): under INVITED_ONLY a
       narrowing, on a shared endpoint required in every mode, ANY included. -->
  {#if lists.directories && preset?.directory_claim}
    <Textarea
      label={t(`app.identity_provider.directories_label_${preset.directory_claim}`)}
      hint={shared
        ? t('app.identity_provider.directories_shared_hint')
        : lists.required
          ? t(`app.identity_provider.directories_hint_${preset.directory_claim}`)
          : t('app.identity_provider.directories_narrow_hint')}
      error={refusal}
      bind:value={draft.directories}
      rows={3}
      spellcheck={false}
      isRequired={lists.required}
    />
  {/if}
  {#if lists.domains}
    <Textarea
      label={t('app.identity_provider.domains_label')}
      hint={t('app.identity_provider.domains_hint')}
      bind:value={draft.domains}
      rows={3}
      spellcheck={false}
    />
  {/if}
  {@render more?.()}
</Stack>
